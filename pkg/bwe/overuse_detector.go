package bwe

import (
	"math"
	"time"
)

const (
	kU = 0.01
	kD = 0.00018

	maxNumDeltas = 60
)

type overuseDetector struct {
	adaptiveThreshold    bool
	overUseTimeThreshold time.Duration
	delayThreshold       float64
	lastEstimate         time.Duration
	lastUpdate           time.Time
	firstOverUse         time.Time
	inOveruse            bool
	lastUsage            usage
}

func newOveruseDetector(adaptive bool) *overuseDetector {
	return &overuseDetector{
		adaptiveThreshold:    adaptive,
		overUseTimeThreshold: 10 * time.Millisecond,
		delayThreshold:       12.5,
		lastEstimate:         0,
		lastUpdate:           time.Time{},
		firstOverUse:         time.Time{},
		inOveruse:            false,
	}
}

func (d *overuseDetector) update(ts time.Time, trend float64, numDeltas int) usage {
	if numDeltas < 2 {
		return usageNormal
	}
	modifiedTrend := float64(min(numDeltas, maxNumDeltas)) * trend

	if modifiedTrend > d.delayThreshold {
		if d.firstOverUse.IsZero() {
			d.firstOverUse = ts
		}
		if ts.Sub(d.firstOverUse) > d.overUseTimeThreshold {
			d.firstOverUse = time.Time{}
			d.lastUsage = usageOver
		}
	} else if modifiedTrend < -d.delayThreshold {
		d.firstOverUse = time.Time{}
		d.lastUsage = usageUnder
	} else {
		d.firstOverUse = time.Time{}
		d.lastUsage = usageNormal
	}
	if d.adaptiveThreshold {
		d.adaptThreshold(ts, modifiedTrend)
	}
	return d.lastUsage
}

func (d *overuseDetector) adaptThreshold(ts time.Time, modifiedTrend float64) {
	if d.lastUpdate.IsZero() {
		d.lastUpdate = ts
	}
	if math.Abs(modifiedTrend) > d.delayThreshold+15 {
		d.lastUpdate = ts
		return
	}
	k := kU
	if math.Abs(modifiedTrend) < d.delayThreshold {
		k = kD
	}
	delta := ts.Sub(d.lastUpdate)
	if delta > 100*time.Millisecond {
		delta = 100 * time.Millisecond
	}
	d.delayThreshold += k * (math.Abs(modifiedTrend) - d.delayThreshold) * float64(delta.Milliseconds())
	d.delayThreshold = max(min(d.delayThreshold, 600.0), 6.0)
	d.lastUpdate = ts
}
