// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"math"
	"time"
)

const (
	maxDeltas = 60
)

type adaptiveThresholdOption func(*adaptiveThreshold)

func setInitialThreshold(t time.Duration) adaptiveThresholdOption {
	return func(at *adaptiveThreshold) {
		at.thresh = t
	}
}

// adaptiveThreshold implements a threshold that continuously adapts depending on
// the current measurements/estimates. This is necessary to avoid starving GCC
// in the presence of concurrent TCP flows by allowing larger Queueing delays,
// when measurements/estimates increase. overuseCoefficientU and
// overuseCoefficientD define by how much the threshold adapts. We basically
// want the threshold to increase fast, if the measurement is outside [-thresh,
// thresh] and decrease slowly if it is within.
//
// See https://datatracker.ietf.org/doc/html/draft-ietf-rmcat-gcc-02#section-5.4
// or [Analysis and Design of the Google Congestion Control for Web Real-time
// Communication (WebRTC)](https://c3lab.poliba.it/images/6/65/Gcc-analysis.pdf)
// for a more detailed description
type adaptiveThreshold struct {
	thresh                 time.Duration
	overuseCoefficientUp   float64
	overuseCoefficientDown float64
	min                    time.Duration
	max                    time.Duration
	lastUpdate             time.Time
	numDeltas              int
}

// newAdaptiveThreshold initializes a new adaptiveThreshold with default
// values taken from draft-ietf-rmcat-gcc-02
func newAdaptiveThreshold(opts ...adaptiveThresholdOption) *adaptiveThreshold {
	at := &adaptiveThreshold{
		thresh:                 time.Duration(12500 * float64(time.Microsecond)),
		overuseCoefficientUp:   0.01,
		overuseCoefficientDown: 0.00018,
		min:                    6 * time.Millisecond,
		max:                    600 * time.Millisecond,
		lastUpdate:             time.Time{},
		numDeltas:              0,
	}
	for _, opt := range opts {
		opt(at)
	}
	return at
}

func (a *adaptiveThreshold) compare(estimate, _ time.Duration) (usage, time.Duration, time.Duration) {
	a.numDeltas++
	if a.numDeltas < 2 {
		return usageNormal, estimate, a.max
	}
	t := time.Duration(minInt(a.numDeltas, maxDeltas)) * estimate
	use := usageNormal
	if t > a.thresh {
		use = usageOver
	} else if t < -a.thresh {
		use = usageUnder
	}
	thresh := a.thresh
	a.update(t)
	return use, t, thresh
}

func (a *adaptiveThreshold) update(estimate time.Duration) {
	now := time.Now()
	if a.lastUpdate.IsZero() {
		a.lastUpdate = now
	}
	absEstimate := time.Duration(math.Abs(float64(estimate.Microseconds()))) * time.Microsecond
	if absEstimate > a.thresh+15*time.Millisecond {
		a.lastUpdate = now
		return
	}
	k := a.overuseCoefficientUp
	if absEstimate < a.thresh {
		k = a.overuseCoefficientDown
	}
	maxTimeDelta := 100 * time.Millisecond
	timeDelta := time.Duration(minInt(int(now.Sub(a.lastUpdate).Milliseconds()), int(maxTimeDelta.Milliseconds()))) * time.Millisecond
	d := absEstimate - a.thresh
	add := k * float64(d.Milliseconds()) * float64(timeDelta.Milliseconds())
	a.thresh += time.Duration(add*1000) * time.Microsecond
	a.thresh = clampDuration(a.thresh, a.min, a.max)
	a.lastUpdate = now
}
