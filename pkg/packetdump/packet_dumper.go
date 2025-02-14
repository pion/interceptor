// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package packetdump

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// ErrBothBinaryAndDeprecatedFormat is returned when both binary and deprecated format callbacks are set.
var ErrBothBinaryAndDeprecatedFormat = fmt.Errorf("both binary and deprecated format callbacks are set")

// PacketDumper dumps packet to a io.Writer.
type PacketDumper struct {
	log logging.LeveledLogger

	wg    sync.WaitGroup
	close chan struct{}

	rtpChan  chan *rtpDump
	rtcpChan chan *rtcpDump

	rtpStream  io.Writer
	rtcpStream io.Writer

	rtpFormatBinary  RTPBinaryFormatCallback
	rtcpFormatBinary RTCPBinaryFormatCallback

	rtpFormat  RTPFormatCallback
	rtcpFormat RTCPFormatCallback

	rtpFilter           RTPFilterCallback
	rtcpFilter          RTCPFilterCallback
	rtcpPerPacketFilter RTCPPerPacketFilterCallback
}

// NewPacketDumper creates a new PacketDumper.
func NewPacketDumper(opts ...PacketDumperOption) (*PacketDumper, error) {
	dumper := &PacketDumper{
		log:              logging.NewDefaultLoggerFactory().NewLogger("packet_dumper"),
		wg:               sync.WaitGroup{},
		close:            make(chan struct{}),
		rtpChan:          make(chan *rtpDump),
		rtcpChan:         make(chan *rtcpDump),
		rtpStream:        os.Stdout,
		rtcpStream:       os.Stdout,
		rtpFormat:        nil,
		rtcpFormat:       nil,
		rtpFormatBinary:  nil,
		rtcpFormatBinary: nil,
		rtpFilter: func(*rtp.Packet) bool {
			return true
		},
		rtcpFilter: func([]rtcp.Packet) bool {
			return true
		},
		rtcpPerPacketFilter: func(rtcp.Packet) bool {
			return true
		},
	}

	if dumper.rtpFormat != nil && dumper.rtpFormatBinary != nil {
		return nil, ErrBothBinaryAndDeprecatedFormat
	}

	for _, opt := range opts {
		if err := opt(dumper); err != nil {
			return nil, err
		}
	}

	if dumper.rtpFormat == nil && dumper.rtpFormatBinary == nil {
		dumper.rtpFormat = DefaultRTPFormatter
	}

	if dumper.rtcpFormat == nil && dumper.rtcpFormatBinary == nil {
		dumper.rtcpFormat = DefaultRTCPFormatter
	}

	dumper.wg.Add(1)
	go dumper.loop()

	return dumper, nil
}

func (d *PacketDumper) logRTPPacket(header *rtp.Header, payload []byte, attributes interceptor.Attributes) {
	select {
	case d.rtpChan <- &rtpDump{
		attributes: attributes,
		packet: &rtp.Packet{
			Header:  *header,
			Payload: payload,
		},
	}:
	case <-d.close:
	}
}

func (d *PacketDumper) logRTCPPackets(pkts []rtcp.Packet, attributes interceptor.Attributes) {
	select {
	case d.rtcpChan <- &rtcpDump{
		attributes: attributes,
		packets:    pkts,
	}:
	case <-d.close:
	}
}

// Close closes the PacketDumper.
func (d *PacketDumper) Close() error {
	defer d.wg.Wait()

	if !d.isClosed() {
		close(d.close)
	}

	return nil
}

func (d *PacketDumper) isClosed() bool {
	select {
	case <-d.close:
		return true
	default:
		return false
	}
}

func (d *PacketDumper) loop() {
	defer d.wg.Done()

	for {
		select {
		case <-d.close:
			return
		case dump := <-d.rtpChan:
			err := d.writeDumpedRTP(dump)
			if err != nil {
				d.log.Errorf("could not dump RTP packet: %v", err)
			}
		case dump := <-d.rtcpChan:
			err := d.writeDumpedRTCP(dump)
			if err != nil {
				d.log.Errorf("could not dump RTCP packets: %v", err)
			}
		}
	}
}

func (d *PacketDumper) writeDumpedRTP(dump *rtpDump) error {
	if !d.rtpFilter(dump.packet) {
		return nil
	}

	if d.rtpFormatBinary != nil {
		dumped, err := d.rtpFormatBinary(dump.packet, dump.attributes)
		if err != nil {
			return fmt.Errorf("rtp format binary: %w", err)
		}
		_, err = d.rtpStream.Write(dumped)
		if err != nil {
			return fmt.Errorf("rtp stream write: %w", err)
		}
	}

	if d.rtpFormat != nil {
		if _, err := fmt.Fprint(d.rtpStream, d.rtpFormat(dump.packet, dump.attributes)); err != nil {
			return fmt.Errorf("rtp stream Fprint: %w", err)
		}
	}

	return nil
}

func (d *PacketDumper) writeDumpedRTCP(dump *rtcpDump) error {
	if !d.rtcpFilter(dump.packets) {
		return nil
	}

	for _, pkt := range dump.packets {
		if !d.rtcpPerPacketFilter(pkt) {
			continue
		}

		if d.rtcpFormatBinary != nil {
			dumped, err := d.rtcpFormatBinary(pkt, dump.attributes)
			if err != nil {
				return fmt.Errorf("rtcp format binary: %w", err)
			}

			_, err = d.rtcpStream.Write(dumped)
			if err != nil {
				return fmt.Errorf("rtcp stream write: %w", err)
			}
		}
	}

	if d.rtcpFormat != nil {
		if _, err := fmt.Fprint(d.rtcpStream, d.rtcpFormat(dump.packets, dump.attributes)); err != nil {
			return fmt.Errorf("rtсp stream Fprint: %w", err)
		}
	}

	return nil
}
