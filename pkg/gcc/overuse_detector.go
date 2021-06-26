package gcc

import (
	"time"
)

type threshold interface {
	compare(estimate time.Duration, delta time.Duration) (usage, time.Duration, time.Duration)
}

type overuseDetector struct {
	threshold   threshold
	overuseTime time.Duration
}

func newOveruseDetector(thresh threshold, overuseTime time.Duration) *overuseDetector {
	return &overuseDetector{
		threshold:   thresh,
		overuseTime: overuseTime,
	}
}

func (d *overuseDetector) run(in <-chan DelayStats) <-chan DelayStats {
	out := make(chan DelayStats)
	go func() {
		lastEstimate := 0 * time.Millisecond
		lastUpdate := time.Now()
		var increasingDuration time.Duration
		var increasingCounter int

		for next := range in {
			now := time.Now()
			delta := now.Sub(lastUpdate)
			lastUpdate = now

			thresholdUse, estimate, currentThreshold := d.threshold.compare(next.Estimate, next.lastReceiveDelta)

			use := usageNormal
			if thresholdUse == usageOver {
				if increasingDuration == 0 {
					increasingDuration = delta / 2
				} else {
					increasingDuration += delta
				}
				increasingCounter++
				if increasingDuration > d.overuseTime && increasingCounter > 1 {
					if estimate > lastEstimate {
						use = usageOver
					}
				}
			}
			if thresholdUse == usageUnder {
				increasingCounter = 0
				increasingDuration = 0
				use = usageUnder
			}

			if thresholdUse == usageNormal {
				increasingDuration = 0
				increasingCounter = 0
				use = usageNormal
			}
			lastEstimate = estimate

			out <- DelayStats{
				Measurement:      next.Measurement,
				Estimate:         estimate,
				Threshold:        currentThreshold,
				lastReceiveDelta: delta,
				Usage:            use,
				State:            0,
				TargetBitrate:    0,
				RTT:              0,
			}
		}
		close(out)
	}()
	return out
}
