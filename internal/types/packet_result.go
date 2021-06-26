package types

import (
	"time"

	"github.com/pion/rtp"
)

type SentPacket struct {
	SendTime time.Time
	Header   *rtp.Header
}

// PacketResult holds information about a packet and if/when it has been
// sent/received.
type PacketResult struct {
	SentPacket  SentPacket
	ReceiveTime time.Time
	Received    bool
}
