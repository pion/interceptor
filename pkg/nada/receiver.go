package nada

import (
	"math"
	"time"
)

// Receiver represents a receiver of a NADA bandwidth estimator.
type Receiver struct {
	config                         Config
	BaselineDelay                  time.Duration // d_base
	EstimatedQueuingDelay          time.Duration // d_queue
	EstimatedPacketLossRatio       float64
	EstimatedPacketECNMarkingRatio float64
	ReceivingRate                  BitsPerSecond
	LastTimestamp                  time.Time
	CurrentTimestamp               time.Time
	RecommendedRateAdaptionMode    RateAdaptionMode

	packetStream *packetStream
}

// NewReceiver creates a new NADA receiver.
func NewReceiver(now time.Time, config Config) *Receiver {
	return &Receiver{
		config:                         config,
		BaselineDelay:                  time.Duration(1<<63 - 1),
		EstimatedPacketLossRatio:       0.0,
		EstimatedPacketECNMarkingRatio: 0.0,
		ReceivingRate:                  0.0,
		LastTimestamp:                  now,
		CurrentTimestamp:               now,
		packetStream:                   newPacketStream(config.LogWindow),
	}
}

// OnReceiveMediaPacket implements the media receive algorithm.
func (r *Receiver) OnReceiveMediaPacket(now time.Time, sent time.Time, seq uint16, ecn bool, size Bits) error {
	// obtain current timestamp t_curr from system clock
	r.CurrentTimestamp = now

	// obtain from packet header sending time stamp tSent
	tSent := sent

	// obtain one-way delay measurement: dFwd = t_curr - t_sent
	dFwd := r.CurrentTimestamp.Sub(tSent)

	// update baseline delay: d_base = min(d_base, d_fwd)
	if dFwd < r.BaselineDelay {
		r.BaselineDelay = dFwd
	}

	// update queuing delay:  d_queue = d_fwd - d_base
	r.EstimatedQueuingDelay = dFwd - r.BaselineDelay

	if err := r.packetStream.add(now, seq, ecn, size, r.EstimatedQueuingDelay > r.config.QueueingDelayThreshold); err != nil {
		return err
	}

	pLossInst, pMarkInst, rRecvInst, hasQueueingDelay := r.packetStream.prune(now)

	// update packet loss ratio estimate p_loss
	// r.config.α*p_loss_inst + (1-r.config.α)*r.EstimatedPacketLossRatio
	r.EstimatedPacketLossRatio = r.config.Alpha*(pLossInst-r.EstimatedPacketLossRatio) + r.EstimatedPacketLossRatio

	// update packet marking ratio estimate p_mark
	// r.config.α*p_mark_inst + (1-r.config.α)*r.EstimatedPacketECNMarkingRatio
	r.EstimatedPacketECNMarkingRatio = r.config.Alpha*(pMarkInst-r.EstimatedPacketECNMarkingRatio) + r.EstimatedPacketECNMarkingRatio

	// update measurement of receiving rate r_recv
	r.ReceivingRate = rRecvInst

	// update recommended rate adaption mode.
	if pLossInst == 0 && !hasQueueingDelay {
		r.RecommendedRateAdaptionMode = RateAdaptionModeAcceleratedRampUp
	} else {
		r.RecommendedRateAdaptionMode = RateAdaptionModeGradualUpdate
	}

	return nil
}

// BuildFeedbackReport creates a new feedback packet.
func (r *Receiver) BuildFeedbackReport() *FeedbackReport {
	// calculate non-linear warping of delay d_tilde if packet loss exists
	equivalentDelay := r.equivalentDelay()

	// calculate current aggregate congestion signal x_curr
	aggregatedCongestionSignal := equivalentDelay +
		scale(r.config.ReferenceDelayMarking, math.Pow(r.EstimatedPacketECNMarkingRatio/r.config.ReferencePacketMarkingRatio, 2)) +
		scale(r.config.ReferenceDelayLoss, math.Pow(r.EstimatedPacketLossRatio/r.config.ReferencePacketLossRatio, 2))

	// determine mode of rate adaptation for sender: rmode
	rmode := r.RecommendedRateAdaptionMode

	// update t_last = t_curr
	r.LastTimestamp = r.CurrentTimestamp

	// send feedback containing values of: rmode, x_curr, and r_recv
	return &FeedbackReport{
		RecommendedRateAdaptionMode: rmode,
		AggregatedCongestionSignal:  aggregatedCongestionSignal,
		ReceivingRate:               r.ReceivingRate,
	}
}

func scale(t time.Duration, x float64) time.Duration {
	return time.Duration(float64(t) * x)
}

// d_tilde computes d_tilde as described by
//
//               / d_queue,                   if d_queue<QTH;
//               |
//    d_tilde = <                                           (1)
//               |                  (d_queue-QTH)
//               \ QTH exp(-LAMBDA ---------------) , otherwise.
//                                     QTH
//
func (r *Receiver) equivalentDelay() time.Duration {
	if r.EstimatedQueuingDelay < r.config.DelayThreshold {
		return r.EstimatedQueuingDelay
	}
	scaling := math.Exp(-r.config.Lambda * float64((r.EstimatedQueuingDelay-r.config.DelayThreshold)/r.config.DelayThreshold))
	return scale(r.config.DelayThreshold, scaling)
}
