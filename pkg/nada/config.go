package nada

import "time"

// Bits represents a unit of one bit.
type Bits uint32

// BitsPerSecond represents a unit of one bit per second.
type BitsPerSecond float64

const (
	// Kbps represents 1 kbps.
	Kbps = BitsPerSecond(1_000)
	// Mbps represents 1 Mbps.
	Mbps = BitsPerSecond(1_000_000)
)

// Config represents the configuration of a NADA bandwidth estimator.
type Config struct {
	// Weight of priority of the flow
	Priority float64
	// Minimum rate of the application supported by the media encoder
	MinimumRate BitsPerSecond // RMIN
	// Maximum rate of the application supported by media encoder
	MaximumRate BitsPerSecond // RMAX
	// Reference congestion level
	ReferenceCongestionLevel time.Duration // XREF
	// Scaling parameter for gradual rate update calculation
	Kappa float64
	// Scaling parameter for gradual rate update calculation
	Eta float64
	// Upper bound of RTT in gradual rate update calculation
	Tau time.Duration
	// Target feedback interval
	Delta time.Duration

	// Observation window in time for calculating packet summary statistics at receiver
	LogWindow time.Duration // LOGWIN
	// Threshold for determining queuing delay build up at receiver
	QueueingDelayThreshold time.Duration
	// Bound on filtering delay
	FilteringDelay time.Duration // DFILT
	// Upper bound on rate increase ratio for accelerated ramp-up
	GammaMax float64
	// Upper bound on self-inflicted queueing delay during ramp up
	QueueBound time.Duration // QBOUND

	// Multiplier for self-scaling the expiration threshold of the last observed loss
	// (loss_exp) based on measured average loss interval (loss_int)
	LossMultiplier float64 // MULTILOSS
	// Delay threshold for invoking non-linear warping
	DelayThreshold time.Duration // QTH
	// Scaling parameter in the exponent of non-linear warping
	Lambda float64

	// Reference packet loss ratio
	ReferencePacketLossRatio float64 // PLRREF
	// Reference packet marking ratio
	ReferencePacketMarkingRatio float64 // PMRREF
	// Reference delay penalty for loss when lacket loss ratio is at least PLRREF
	ReferenceDelayLoss time.Duration // DLOSS
	// Reference delay penalty for ECN marking when packet marking is at PMRREF
	ReferenceDelayMarking time.Duration // DMARK

	// Frame rate of incoming video
	FrameRate float64 // FRAMERATE
	// Scaling parameter for modulating outgoing sending rate
	BetaSending float64
	// Scaling parameter for modulating video encoder target rate
	BetaVideoEncoder float64
	// Smoothing factor in exponential smoothing of packet loss and marking rate
	Alpha float64
}

// DefaultConfig returns the default configuration recommended by the specification.
func DefaultConfig() Config {
	return Config{
		Priority:                 1.0,
		MinimumRate:              150 * Kbps,
		MaximumRate:              1500 * Kbps,
		ReferenceCongestionLevel: 10 * time.Millisecond,
		Kappa:                    0.5,
		Eta:                      2.0,
		Tau:                      500 * time.Millisecond,
		Delta:                    100 * time.Millisecond,

		LogWindow:              500 * time.Millisecond,
		QueueingDelayThreshold: 10 * time.Millisecond,
		FilteringDelay:         120 * time.Millisecond,
		GammaMax:               0.5,
		QueueBound:             50 * time.Millisecond,

		LossMultiplier: 7.0,
		DelayThreshold: 50 * time.Millisecond,
		Lambda:         0.5,

		ReferencePacketLossRatio:    0.01,
		ReferencePacketMarkingRatio: 0.01,
		ReferenceDelayLoss:          10 * time.Millisecond,
		ReferenceDelayMarking:       2 * time.Millisecond,

		FrameRate:        30.0,
		BetaSending:      0.1,
		BetaVideoEncoder: 0.1,
		Alpha:            0.1,
	}
}
