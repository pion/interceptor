package gcc

import (
	"math"
	"time"

	"github.com/pion/interceptor/internal/types"
	"github.com/pion/logging"
)

type lossBasedBandwidthEstimator struct {
	bitrate        types.DataRate
	averageLoss    float64
	lastLossUpdate time.Time
	lastIncrease   time.Time
	lastDecrease   time.Time
	inertia        float64
	decay          float64
	log            logging.LeveledLogger
}

func newLossBasedBWE() *lossBasedBandwidthEstimator {
	return &lossBasedBandwidthEstimator{
		inertia:     0.5,
		decay:       0.5,
		bitrate:     0,
		averageLoss: 0,
		log:         logging.NewDefaultLoggerFactory().NewLogger("gcc_loss_controller"),
	}
}

func (e *lossBasedBandwidthEstimator) getEstimate(wantedRate types.DataRate) types.DataRate {
	if e.bitrate <= 0 {
		e.bitrate = wantedRate
	}

	return e.bitrate
}

func (e *lossBasedBandwidthEstimator) updateLossStats(results []types.PacketResult) {

	if len(results) == 0 {
		return
	}

	packetsLost := 0
	for _, p := range results {
		if !p.Received {
			packetsLost++
		}
	}

	lossRatio := float64(packetsLost) / float64(len(results))
	//e.averageLoss = e.inertia*lossRatio + e.decay*(1-e.inertia)*e.averageLoss
	e.averageLoss = e.average(time.Since(e.lastLossUpdate), e.averageLoss, lossRatio)
	e.lastLossUpdate = time.Now()

	increaseLoss := math.Max(e.averageLoss, lossRatio)
	decreaseLoss := math.Min(e.averageLoss, lossRatio)

	e.log.Infof("averageLoss: %v", e.averageLoss)

	// Naive implementation using constants from IETF Draft
	// TODO(mathis): Make this more smart and configurable. (Smart here means
	// don't decrease too often and such things, see libwebrtc)
	if increaseLoss < 0.02 && time.Since(e.lastIncrease) > 200*time.Millisecond {
		e.lastIncrease = time.Now()
		e.bitrate = types.DataRate(1.05 * float64(e.bitrate))
	} else if decreaseLoss > 0.1 && time.Since(e.lastDecrease) > 200*time.Millisecond {
		e.lastDecrease = time.Now()
		e.bitrate = types.DataRate(float64(e.bitrate) * (1 - 0.5*e.averageLoss))
	}
}

func (e *lossBasedBandwidthEstimator) average(delta time.Duration, prev, sample float64) float64 {
	return sample + math.Exp(-float64(delta.Milliseconds())/200.0)*(prev-sample)
}
