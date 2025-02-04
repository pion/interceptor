package bwe

import (
	"fmt"
	"time"
)

type ECN uint8

const (
	//nolint:misspell
	// ECNNonECT signals Non ECN-Capable Transport, Non-ECT
	ECNNonECT ECN = iota // 00

	//nolint:misspell
	// ECNECT1 signals ECN Capable Transport, ECT(0)
	ECNECT1 // 01

	//nolint:misspell
	// ECNECT0 signals ECN Capable Transport, ECT(1)
	ECNECT0 // 10

	// ECNCE signals ECN Congestion Encountered, CE
	ECNCE // 11
)

type Acknowledgment struct {
	SeqNr     int64
	Size      uint16
	Departure time.Time
	Arrived   bool
	Arrival   time.Time
	ECN       ECN
}

func (a Acknowledgment) String() string {
	return fmt.Sprintf("seq=%v, departure=%v, arrival=%v", a.SeqNr, a.Departure, a.Arrival)
}
