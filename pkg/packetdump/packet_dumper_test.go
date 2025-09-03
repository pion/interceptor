// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package packetdump

import (
	"testing"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

type customLogger struct {
	rtpLog  chan rtpDump
	rtcpLog chan rtcpDump
}

// LogRTCPPackets implements PacketLogger.
func (c *customLogger) LogRTCPPackets(pkts []rtcp.Packet, attributes interceptor.Attributes) {
	c.rtcpLog <- rtcpDump{
		attributes: attributes,
		packets:    pkts,
	}
}

// LogRTPPacket implements PacketLogger.
func (c *customLogger) LogRTPPacket(header *rtp.Header, payload []byte, attributes interceptor.Attributes) {
	c.rtpLog <- rtpDump{
		attributes: attributes,
		packet: &rtp.Packet{
			Header:  *header,
			Payload: payload,
		},
	}
}

func TestCustomLogger(t *testing.T) {
	cl := &customLogger{
		rtpLog:  make(chan rtpDump, 1),
		rtcpLog: make(chan rtcpDump, 1),
	}
	dumper, err := NewPacketDumper(PacketLog(cl))
	assert.NoError(t, err)
	dumper.logRTPPacket(&rtp.Header{}, []byte{1, 2, 3, 4}, nil)
	dumper.logRTCPPackets([]rtcp.Packet{
		&rtcp.RawPacket{0, 1, 2, 3},
	}, nil)

	rtpL := <-cl.rtpLog
	assert.Equal(t, rtpDump{
		attributes: nil,
		packet: &rtp.Packet{
			Header:  rtp.Header{},
			Payload: []byte{1, 2, 3, 4},
		},
	}, rtpL)
	rtcpL := <-cl.rtcpLog
	assert.Equal(t, rtcpDump{
		attributes: nil,
		packets: []rtcp.Packet{
			&rtcp.RawPacket{0, 1, 2, 3},
		},
	}, rtcpL)
}
