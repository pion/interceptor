package packetdump

import (
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// RTPFilterCallback can be used to filter RTP packets to dump.
// The callback returns whether or not to print dump the packet's content.
type RTPFilterCallback func(pkt *rtp.Packet) bool

// RTCPFilterCallback can be used to filter RTCP packets to dump.
// The callback returns whether or not to print dump the packet's content.
type RTCPFilterCallback func(pkt *rtcp.Packet) bool
