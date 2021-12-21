package gcc

import (
	"math"
	"sync"
	"time"

	"github.com/pion/logging"
)

type LossStats struct {
	TargetBitrate int
}

type lossBasedBandwidthEstimator struct {
	lock           sync.Mutex
	maxBitrate     int
	minBitrate     int
	bitrate        int
	averageLoss    float64
	lastLossUpdate time.Time
	lastIncrease   time.Time
	lastDecrease   time.Time
	log            logging.LeveledLogger
}

func newLossBasedBWE(initialBitrate int) *lossBasedBandwidthEstimator {
	return &lossBasedBandwidthEstimator{
		lock:           sync.Mutex{},
		maxBitrate:     100_000_000, // 100 mbit
		minBitrate:     100_000,     // 100 kbit
		bitrate:        initialBitrate,
		averageLoss:    0,
		lastLossUpdate: time.Time{},
		lastIncrease:   time.Time{},
		lastDecrease:   time.Time{},
		log:            logging.NewDefaultLoggerFactory().NewLogger("gcc_loss_controller"),
	}
}

func (e *lossBasedBandwidthEstimator) getEstimate(wantedRate int) LossStats {
	e.lock.Lock()
	defer e.lock.Unlock()

	if e.bitrate <= 0 {
		e.bitrate = min(max(wantedRate, e.minBitrate), e.maxBitrate)
	}
	e.bitrate = min(wantedRate, e.bitrate)

	return LossStats{
		TargetBitrate: e.bitrate,
	}
}

func (e *lossBasedBandwidthEstimator) updateLossEstimate(results []Acknowledgment) {
	if len(results) == 0 {
		return
	}

	packetsLost := 0
	for _, p := range results {
		if p.Arrival.IsZero() {
			packetsLost++
		}
	}

	lossRatio := float64(packetsLost) / float64(len(results))
	e.averageLoss = e.average(time.Since(e.lastLossUpdate), e.averageLoss, lossRatio)
	e.lastLossUpdate = time.Now()

	increaseLoss := math.Max(e.averageLoss, lossRatio)
	decreaseLoss := math.Min(e.averageLoss, lossRatio)

	e.log.Infof("averageLoss: %v, decreaseLoss: %v, increaseLoss: %v", e.averageLoss, decreaseLoss, increaseLoss)

	e.lock.Lock()
	defer e.lock.Unlock()

	if increaseLoss < 0.02 && time.Since(e.lastIncrease) > 200*time.Millisecond {
		e.lastIncrease = time.Now()
		e.bitrate = min(int(1.05*float64(e.bitrate)), e.maxBitrate)
	} else if decreaseLoss > 0.1 && time.Since(e.lastDecrease) > 200*time.Millisecond {
		e.lastDecrease = time.Now()
		e.bitrate = max(int(float64(e.bitrate)*(1-0.5*decreaseLoss)), e.minBitrate)
	}
}

func (e *lossBasedBandwidthEstimator) average(delta time.Duration, prev, sample float64) float64 {
	return sample + math.Exp(-float64(delta.Milliseconds())/200.0)*(prev-sample)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
