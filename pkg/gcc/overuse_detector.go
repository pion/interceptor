// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

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

	dsWriter func(DelayStats)

	lastEstimate       time.Duration
	lastUpdate         time.Time
	increasingDuration time.Duration
	increasingCounter  int
}

func newOveruseDetector(thresh threshold, overuseTime time.Duration, dsw func(DelayStats)) *overuseDetector {
	return &overuseDetector{
		threshold:          thresh,
		overuseTime:        overuseTime,
		dsWriter:           dsw,
		lastEstimate:       0,
		lastUpdate:         time.Now(),
		increasingDuration: 0,
		increasingCounter:  0,
	}
}

func (d *overuseDetector) onDelayStats(ds DelayStats) {
	now := time.Now()
	delta := now.Sub(d.lastUpdate)
	d.lastUpdate = now

	thresholdUse, estimate, currentThreshold := d.threshold.compare(ds.Estimate, ds.LastReceiveDelta)

	use := usageNormal
	if thresholdUse == usageOver {
		if d.increasingDuration == 0 {
			d.increasingDuration = delta / 2
		} else {
			d.increasingDuration += delta
		}
		d.increasingCounter++
		if d.increasingDuration > d.overuseTime && d.increasingCounter > 1 {
			if estimate > d.lastEstimate {
				use = usageOver
			}
		}
	}
	if thresholdUse == usageUnder {
		d.increasingCounter = 0
		d.increasingDuration = 0
		use = usageUnder
	}

	if thresholdUse == usageNormal {
		d.increasingDuration = 0
		d.increasingCounter = 0
		use = usageNormal
	}
	d.lastEstimate = estimate

	d.dsWriter(DelayStats{
		Measurement:      ds.Measurement,
		Estimate:         estimate,
		Threshold:        currentThreshold,
		LastReceiveDelta: ds.LastReceiveDelta,
		Usage:            use,
		State:            0,
		TargetBitrate:    0,
	})
}
