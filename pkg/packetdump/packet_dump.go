// Package packetdump implements RTP & RTCP packet dumpers.
package packetdump

import (
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

type rtpDump struct {
	packet *rtp.Packet
}

type rtcpDump struct {
	packets []rtcp.Packet
}
