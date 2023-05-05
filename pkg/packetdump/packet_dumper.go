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

// PacketDumper dumps packet to a io.Writer
type PacketDumper struct {
	log logging.LeveledLogger

	wg    sync.WaitGroup
	close chan struct{}

	rtpChan  chan *rtpDump
	rtcpChan chan *rtcpDump

	rtpStream  io.Writer
	rtcpStream io.Writer

	rtpFormat  RTPFormatCallback
	rtcpFormat RTCPFormatCallback

	rtpFilter  RTPFilterCallback
	rtcpFilter RTCPFilterCallback
}

// NewPacketDumper creates a new PacketDumper
func NewPacketDumper(opts ...PacketDumperOption) (*PacketDumper, error) {
	d := &PacketDumper{
		log:        logging.NewDefaultLoggerFactory().NewLogger("packet_dumper"),
		wg:         sync.WaitGroup{},
		close:      make(chan struct{}),
		rtpChan:    make(chan *rtpDump),
		rtcpChan:   make(chan *rtcpDump),
		rtpStream:  os.Stdout,
		rtcpStream: os.Stdout,
		rtpFormat:  DefaultRTPFormatter,
		rtcpFormat: DefaultRTCPFormatter,
		rtpFilter: func(pkt *rtp.Packet) bool {
			return true
		},
		rtcpFilter: func(pkt []rtcp.Packet) bool {
			return true
		},
	}

	for _, opt := range opts {
		if err := opt(d); err != nil {
			return nil, err
		}
	}

	d.wg.Add(1)
	go d.loop()

	return d, nil
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

// Close closes the PacketDumper
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
			if d.rtpFilter(dump.packet) {
				if _, err := fmt.Fprint(d.rtpStream, d.rtpFormat(dump.packet, dump.attributes)); err != nil {
					d.log.Errorf("could not dump RTP packet %v", err)
				}
			}
		case dump := <-d.rtcpChan:
			if d.rtcpFilter(dump.packets) {
				if _, err := fmt.Fprint(d.rtcpStream, d.rtcpFormat(dump.packets, dump.attributes)); err != nil {
					d.log.Errorf("could not dump RTCP packet %v", err)
				}
			}
		}
	}
}
