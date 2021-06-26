package gcc

import (
	"math"
	"time"

	"github.com/pion/logging"
)

type rateController struct {
	log                  logging.LeveledLogger
	now                  now
	initialTargetBitrate int
	minBitrate           int
	maxBitrate           int

	target             int
	lastUpdate         time.Time
	lastState          state
	latestRTT          time.Duration
	latestReceivedRate int
	latestDecreaseRate *exponentialMovingAverage
}

type exponentialMovingAverage struct {
	average      float64
	variance     float64
	stdDeviation float64
}

func (a *exponentialMovingAverage) update(value float64) {
	if a.average == 0.0 {
		a.average = value
	} else {
		x := value - a.average
		a.average += decreaseEMAAlpha * x
		a.variance = (1 - decreaseEMAAlpha) * (a.variance + decreaseEMAAlpha*x*x)
		a.stdDeviation = math.Sqrt(a.variance)
	}
}

func newRateController(now now, initialTargetBitrate, minBitrate, maxBitrate int) *rateController {
	return &rateController{
		log:                  logging.NewDefaultLoggerFactory().NewLogger("gcc_rate_controller"),
		now:                  now,
		initialTargetBitrate: initialTargetBitrate,
		minBitrate:           minBitrate,
		maxBitrate:           maxBitrate,
		target:               initialTargetBitrate,
		lastUpdate:           time.Time{},
		lastState:            stateIncrease,
		latestRTT:            0,
		latestReceivedRate:   0,
		latestDecreaseRate:   &exponentialMovingAverage{},
	}
}

func (c *rateController) run(in <-chan DelayStats, receivedRate <-chan int, rtt <-chan time.Duration) chan DelayStats {
	out := make(chan DelayStats)
	go func() {
		c.lastUpdate = c.now()

		defer func() {
			close(out)
		}()

		var latestStats DelayStats
		init := false

		for {
			select {
			case c.latestReceivedRate = <-receivedRate:
			case c.latestRTT = <-rtt:
			case nextStats, ok := <-in:
				if !ok {
					return
				}
				if !init {
					init = true
					latestStats = nextStats
					latestStats.State = stateIncrease
					continue
				}
				latestStats = nextStats
				latestStats.State = latestStats.State.transition(nextStats.Usage)

				now := time.Now()
				switch latestStats.State {
				case stateHold:
				case stateIncrease:
					c.target = clampInt(c.increase(now), c.minBitrate, c.maxBitrate)
					out <- DelayStats{
						Measurement:      latestStats.Measurement,
						Estimate:         latestStats.Estimate,
						Threshold:        latestStats.Threshold,
						lastReceiveDelta: latestStats.lastReceiveDelta,
						Usage:            latestStats.Usage,
						State:            latestStats.State,
						TargetBitrate:    c.target,
						RTT:              c.latestRTT,
					}

				case stateDecrease:
					c.target = clampInt(c.decrease(), c.minBitrate, c.maxBitrate)
					out <- DelayStats{
						Measurement:      latestStats.Measurement,
						Estimate:         latestStats.Estimate,
						Threshold:        latestStats.Threshold,
						lastReceiveDelta: latestStats.lastReceiveDelta,
						Usage:            latestStats.Usage,
						State:            latestStats.State,
						TargetBitrate:    c.target,
						RTT:              c.latestRTT,
					}
				}
			}
		}
	}()
	return out
}

func (c *rateController) increase(now time.Time) int {
	if c.latestDecreaseRate.average > 0 && float64(c.latestReceivedRate) > c.latestDecreaseRate.average-3*c.latestDecreaseRate.stdDeviation &&
		float64(c.latestReceivedRate) < c.latestDecreaseRate.average+3*c.latestDecreaseRate.stdDeviation {
		bitsPerFrame := float64(c.target) / 30.0
		packetsPerFrame := math.Ceil(bitsPerFrame / (1200 * 8))
		expectedPacketSizeBits := bitsPerFrame / packetsPerFrame

		responseTime := 100*time.Millisecond + c.latestRTT
		alpha := 0.5 * math.Min(float64(now.Sub(c.lastUpdate).Milliseconds())/float64(responseTime.Milliseconds()), 1.0)
		increase := int(math.Max(1000.0, alpha*expectedPacketSizeBits))
		c.lastUpdate = now
		return int(math.Min(float64(c.target+increase), 1.5*float64(c.latestReceivedRate)))
	}
	eta := math.Pow(1.08, math.Min(float64(now.Sub(c.lastUpdate).Milliseconds())/1000, 1.0))
	c.lastUpdate = now

	rate := int(eta * float64(c.target))

	// maximum increase to 1.5 * received rate
	received := int(1.5 * float64(c.latestReceivedRate))
	if rate > received && received > c.target {
		return received
	}

	if rate < c.target {
		return c.target
	}
	return rate
}

func (c *rateController) decrease() int {
	target := int(beta * float64(c.latestReceivedRate))
	c.latestDecreaseRate.update(float64(c.latestReceivedRate))
	c.lastUpdate = c.now()
	return target
}
