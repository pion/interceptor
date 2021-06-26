package gcc

import (
	"math"

	"github.com/pion/interceptor/internal/types"
)

type delayBasedBandwidthEstimator struct{}

func (e *delayBasedBandwidthEstimator) getEstimate() types.DataRate {
	return math.MaxInt
}
