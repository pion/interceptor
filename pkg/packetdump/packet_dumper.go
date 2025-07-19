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
	packetLogger PacketLogger

	// Default Logger Options
	log logging.LeveledLogger

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
		packetLogger:     nil,
		log:              logging.NewDefaultLoggerFactory().NewLogger("packet_dumper"),
		rtpStream:        os.Stdout,
		rtcpStream:       os.Stdout,
		rtpFormatBinary:  nil,
		rtcpFormatBinary: nil,
		rtpFormat:        nil,
		rtcpFormat:       nil,
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

	// If we get a custom packet logger, we don't need to set any default logger
	// options.
	if dumper.packetLogger != nil {
		return dumper, nil
	}

	if dumper.rtpFormat == nil && dumper.rtpFormatBinary == nil {
		dumper.rtpFormat = DefaultRTPFormatter
	}

	if dumper.rtcpFormat == nil && dumper.rtcpFormatBinary == nil {
		dumper.rtcpFormat = DefaultRTCPFormatter
	}

	dpl := &defaultPacketLogger{
		log:                 dumper.log,
		wg:                  sync.WaitGroup{},
		close:               make(chan struct{}),
		rtpChan:             make(chan *rtpDump),
		rtcpChan:            make(chan *rtcpDump),
		rtpStream:           dumper.rtpStream,
		rtcpStream:          dumper.rtcpStream,
		rtpFormatBinary:     dumper.rtpFormatBinary,
		rtcpFormatBinary:    dumper.rtcpFormatBinary,
		rtpFormat:           dumper.rtpFormat,
		rtcpFormat:          dumper.rtcpFormat,
		rtpFilter:           dumper.rtpFilter,
		rtcpFilter:          dumper.rtcpFilter,
		rtcpPerPacketFilter: dumper.rtcpPerPacketFilter,
	}
	dpl.run()
	dumper.packetLogger = dpl

	return dumper, nil
}

func (d *PacketDumper) logRTPPacket(header *rtp.Header, payload []byte, attributes interceptor.Attributes) {
	d.packetLogger.LogRTPPacket(header, payload, attributes)
}

func (d *PacketDumper) logRTCPPackets(pkts []rtcp.Packet, attributes interceptor.Attributes) {
	d.packetLogger.LogRTCPPackets(pkts, attributes)
}

func (d *PacketDumper) Close() error {
	dpl, ok := d.packetLogger.(*defaultPacketLogger)
	if ok {
		return dpl.Close()
	}

	return nil
}
