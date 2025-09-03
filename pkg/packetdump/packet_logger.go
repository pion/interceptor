// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package packetdump

import (
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// PacketLogger logs RTP and RTCP Packets.
type PacketLogger interface {
	LogRTPPacket(header *rtp.Header, payload []byte, attributes interceptor.Attributes)
	LogRTCPPackets(pkts []rtcp.Packet, attributes interceptor.Attributes)
}
