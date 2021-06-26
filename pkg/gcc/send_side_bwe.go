package gcc

import (
	"time"

	"github.com/pion/interceptor/internal/types"
)

// SendSideBandwidthEstimator implements send side bandwidth estimation
type SendSideBandwidthEstimator struct {
	lastBWE    types.DataRate
	lossBased  *lossBasedBandwidthEstimator
	delayBased *delayBasedBandwidthEstimator
}

// NewSendSideBandwidthEstimator returns a new send side bandwidth estimator
// using delay based and loss based bandwidth estimation.
func NewSendSideBandwidthEstimator(initialBitrate types.DataRate) *SendSideBandwidthEstimator {
	return &SendSideBandwidthEstimator{
		lastBWE:    initialBitrate,
		lossBased:  newLossBasedBWE(),
		delayBased: &delayBasedBandwidthEstimator{},
	}
}

// OnPacketSent records a packet as sent.
func (g *SendSideBandwidthEstimator) OnPacketSent(ts time.Time, sizeInBytes int) {
}

// OnFeedback updates the GCC statistics from the incoming feedback.
func (g *SendSideBandwidthEstimator) OnFeedback(feedback []types.PacketResult) {
	g.lossBased.updateLossStats(feedback)
}

// GetBandwidthEstimation returns the estimated bandwidth available
func (g *SendSideBandwidthEstimator) GetBandwidthEstimation() types.DataRate {
	return types.MinDataRate(g.delayBased.getEstimate(), g.lossBased.getEstimate(g.lastBWE))
}
