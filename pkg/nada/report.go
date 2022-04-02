package nada

import (
	"encoding/binary"
	"errors"
	"time"
)

// RateAdaptionMode is the receiver-suggested rate mode.
type RateAdaptionMode bool

const (
	// RateAdaptionModeAcceleratedRampUp tells the sender to ramp up faster.
	RateAdaptionModeAcceleratedRampUp = false
	// RateAdaptionModeGradualUpdate tells the sender to ramp up slower.
	RateAdaptionModeGradualUpdate = true
)

// FeedbackReport represents the NADA feedback report sent by the receiver
type FeedbackReport struct {
	// a 1-bit flag indicating
	// whether the sender should operate in accelerated ramp-up mode
	// (rmode=0) or gradual update mode (rmode=1).
	RecommendedRateAdaptionMode RateAdaptionMode

	// the most recently updated
	// value, calculated by the receiver according to Section 4.2.  This
	// information can be expressed with a unit of 100 microsecond (i.e.,
	// 1/10 of a millisecond) in 15 bits.  This allows a maximum value of
	// x_curr at approximately 3.27 second.
	AggregatedCongestionSignal time.Duration // x_curr

	// the most recently measured receiving rate
	// according to Section 5.1.3.  This information is expressed with a
	// unit of bits per second (bps) in 32 bits (unsigned int).  This
	// allows a maximum rate of approximately 4.3Gbps, approximately 1000
	// times of the streaming rate of a typical high-definition (HD)
	// video conferencing session today.  This field can be expanded
	// further by a few more bytes, in case an even higher rate need to
	// be specified.
	ReceivingRate BitsPerSecond // r_recv
}

var errInvalidReport = errors.New("nada report: invalid report state")

// Marshal converts the report to a byte slice
func (r FeedbackReport) Marshal() ([]byte, error) {
	rawPacket := make([]byte, 6)
	xCurr := (r.AggregatedCongestionSignal.Microseconds() / 100)
	if xCurr >= (1>>15) || xCurr < 0 {
		return nil, errInvalidReport
	}
	binary.BigEndian.PutUint16(rawPacket[0:2], uint16(xCurr))
	if r.ReceivingRate >= (1<<32) || r.ReceivingRate < 0 {
		return nil, errInvalidReport
	}
	binary.BigEndian.PutUint32(rawPacket[2:6], uint32(r.ReceivingRate))
	if r.RecommendedRateAdaptionMode {
		rawPacket[0] |= 1 << 7
	}
	return rawPacket, nil
}

// Unmarshal creates a report given a byte slice
func (r *FeedbackReport) Unmarshal(rawPacket []byte) error {
	if len(rawPacket) != 6 {
		return errInvalidReport
	}
	r.RecommendedRateAdaptionMode = (rawPacket[0] & (1 << 7)) != 0
	xCurr := binary.BigEndian.Uint16(rawPacket[0:2])
	xCurr &= 0x7FFF
	r.AggregatedCongestionSignal = time.Duration(xCurr) * 100 * time.Microsecond
	r.ReceivingRate = BitsPerSecond(binary.BigEndian.Uint32(rawPacket[2:6]))
	return nil
}
