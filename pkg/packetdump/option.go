// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package packetdump

import (
	"io"

	"github.com/pion/logging"
)

// PacketDumperOption can be used to configure SenderInterceptor
type PacketDumperOption func(d *PacketDumper) error

// Log sets a logger for the interceptor
func Log(log logging.LeveledLogger) PacketDumperOption {
	return func(d *PacketDumper) error {
		d.log = log
		return nil
	}
}

// RTPWriter sets the io.Writer on which RTP packets will be dumped.
func RTPWriter(w io.Writer) PacketDumperOption {
	return func(d *PacketDumper) error {
		d.rtpStream = w
		return nil
	}
}

// RTCPWriter sets the io.Writer on which RTCP packets will be dumped.
func RTCPWriter(w io.Writer) PacketDumperOption {
	return func(d *PacketDumper) error {
		d.rtcpStream = w
		return nil
	}
}

// RTPFormatter sets the RTP format
// Deprecated: prefer RTPBinaryFormatter
func RTPFormatter(f RTPFormatCallback) PacketDumperOption {
	return func(d *PacketDumper) error {
		d.rtpFormat = f
		return nil
	}
}

// RTCPFormatter sets the RTCP format
// Deprecated: prefer RTCPBinaryFormatter
func RTCPFormatter(f RTCPFormatCallback) PacketDumperOption {
	return func(d *PacketDumper) error {
		d.rtcpFormat = f
		return nil
	}
}

// RTPBinaryFormatter sets the RTP binary formatter
func RTPBinaryFormatter(f RTPBinaryFormatCallback) PacketDumperOption {
	return func(d *PacketDumper) error {
		d.rtpFormatBinary = f
		return nil
	}
}

// RTCPBinaryFormatter sets the RTCP binary formatter
func RTCPBinaryFormatter(f RTCPBinaryFormatCallback) PacketDumperOption {
	return func(d *PacketDumper) error {
		d.rtcpFormatBinary = f
		return nil
	}
}

// RTPFilter sets the RTP filter.
func RTPFilter(callback RTPFilterCallback) PacketDumperOption {
	return func(d *PacketDumper) error {
		d.rtpFilter = callback
		return nil
	}
}

// RTCPFilter sets the RTCP filter.
// Deprecated: prefer RTCPFilterPerPacket
func RTCPFilter(callback RTCPFilterCallback) PacketDumperOption {
	return func(d *PacketDumper) error {
		d.rtcpFilter = callback
		return nil
	}
}

// RTCPFilterPerPacket sets the RTCP per-packet filter.
func RTCPFilterPerPacket(callback RTCPFilterPerPacketCallback) PacketDumperOption {
	return func(d *PacketDumper) error {
		d.rtcpFilterPerPacket = callback
		return nil
	}
}
