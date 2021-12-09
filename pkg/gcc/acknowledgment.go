package gcc

import (
	"time"

	"github.com/pion/rtp"
)

// Acknowledgment holds information about a packet and if/when it has been
// sent/received.
type Acknowledgment struct {
	Header    *rtp.Header
	Size      int
	Departure time.Time
	Arrival   time.Time
	RTT       time.Duration
}
