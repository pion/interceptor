package nada

import (
	"math"
	"time"
)

// Sender represents a NADA sender.
type Sender struct {
	config                            Config
	ReferenceRate                     BitsPerSecond // r_ref
	SenderEstimatedRoundTripTime      time.Duration // rtt
	PreviousAggregateCongestionSignal time.Duration // x_prev
	LastTimestamp                     time.Time
	CurrentTimestamp                  time.Time
}

// NewSender creates a new sender in the NADA estimation pair.
func NewSender(now time.Time, config Config) *Sender {
	return &Sender{
		config:                            config,
		ReferenceRate:                     config.MinimumRate,
		SenderEstimatedRoundTripTime:      0,
		PreviousAggregateCongestionSignal: 0,
		LastTimestamp:                     now,
		CurrentTimestamp:                  now,
	}
}

// UpdateEstimatedRoundTripTime sets the estimated round trip time from an external source.
// This can be calculated from the sender report and receiver reports on the sender.
//
// See https://github.com/versatica/mediasoup/issues/73
func (s *Sender) UpdateEstimatedRoundTripTime(rtt time.Duration) {
	s.SenderEstimatedRoundTripTime = time.Duration((s.config.Alpha * float64(s.SenderEstimatedRoundTripTime)) + ((1 - s.config.Alpha) * float64(rtt)))
}

// OnReceiveFeedbackReport updates the sender with the given NADA feedback report.
func (s *Sender) OnReceiveFeedbackReport(now time.Time, report *FeedbackReport) {
	// obtain current timestamp from system clock: t_curr
	s.CurrentTimestamp = now

	// measure feedback interval: delta = t_curr - t_last
	delta := s.CurrentTimestamp.Sub(s.LastTimestamp)

	if report.RecommendedRateAdaptionMode {
		// update r_ref following gradual update rules
		//
		// In gradual update mode, the rate r_ref is updated as:
		//
		//    x_offset = x_curr - PRIO*XREF*RMAX/r_ref          (5)
		//
		//    x_diff   = x_curr - x_prev                        (6)
		//
		//                           delta    x_offset
		//    r_ref = r_ref - KAPPA*-------*------------*r_ref
		//                            TAU       TAU
		//
		//                                x_diff
		//                  - KAPPA*ETA*---------*r_ref         (7)
		//                                 TAU

		xOffset := report.AggregatedCongestionSignal - scale(s.config.ReferenceCongestionLevel, s.config.Priority*float64(s.config.MaximumRate)/float64(s.ReferenceRate))
		xDiff := report.AggregatedCongestionSignal - s.PreviousAggregateCongestionSignal

		s.ReferenceRate = BitsPerSecond(float64(s.ReferenceRate) *
			(1 -
				(s.config.Kappa * (float64(delta) / float64(s.config.Tau)) * (float64(xOffset) / float64(s.config.Tau))) -
				(s.config.Kappa * s.config.Eta * (float64(xDiff) / float64(s.config.Tau)))))
	} else {
		// update r_ref following accelerated ramp-up rules
		//
		// In accelerated ramp-up mode, the rate r_ref is updated as follows:
		//
		//                                    QBOUND
		//        gamma = min(GAMMA_MAX, ------------------)     (3)
		//                                rtt+DELTA+DFILT
		//
		//        r_ref = max(r_ref, (1+gamma) r_recv)           (4)

		gamma := math.Min(s.config.GammaMax, float64(s.config.QueueBound)/float64(s.SenderEstimatedRoundTripTime+s.config.Delta+s.config.FilteringDelay))
		s.ReferenceRate = BitsPerSecond(math.Max(float64(s.ReferenceRate), (1+gamma)*float64(report.ReceivingRate)))
	}

	// clip rate r_ref within the range of minimum rate (RMIN) and maximum rate (RMAX).
	if s.ReferenceRate < s.config.MinimumRate {
		s.ReferenceRate = s.config.MinimumRate
	}
	if s.ReferenceRate > s.config.MaximumRate {
		s.ReferenceRate = s.config.MaximumRate
	}

	// x_prev = x_curr
	s.PreviousAggregateCongestionSignal = report.AggregatedCongestionSignal

	// t_last = t_curr
	s.LastTimestamp = s.CurrentTimestamp
}

// GetTargetRate returns the target rate for the sender.
func (s *Sender) GetTargetRate(bufferLen uint) BitsPerSecond {
	// r_diff_v = min(0.05*r_ref, BETA_V*8*buffer_len*FPS).     (11)
	// r_vin  = max(RMIN, r_ref - r_diff_v).      (13)

	rDiffV := math.Min(0.05*float64(s.ReferenceRate), s.config.BetaVideoEncoder*8*float64(bufferLen)*(s.config.FrameRate))
	rVin := math.Max(float64(s.config.MinimumRate), float64(s.ReferenceRate)-rDiffV)
	return BitsPerSecond(rVin)
}

// GetSendingRate returns the sending rate for the sender.
func (s *Sender) GetSendingRate(bufferLen uint) BitsPerSecond {
	// r_diff_s = min(0.05*r_ref, BETA_S*8*buffer_len*FPS).     (12)
	// r_send = min(RMAX, r_ref + r_diff_s).    (14)

	rDiffS := math.Min(0.05*float64(s.ReferenceRate), s.config.BetaSending*8*float64(bufferLen)*(s.config.FrameRate))
	rSend := math.Min(float64(s.config.MaximumRate), float64(s.ReferenceRate)+rDiffS)
	return BitsPerSecond(rSend)
}
