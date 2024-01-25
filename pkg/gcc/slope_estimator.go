// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"time"
)

type estimator interface {
	updateEstimate(measurement, lastReceiveDelta time.Duration) time.Duration
}

type slopeEstimator struct {
	estimator
	init             bool
	group            arrivalGroup
	delayStatsWriter func(DelayStats)
}

func newSlopeEstimator(e estimator, dsw func(DelayStats)) *slopeEstimator {
	return &slopeEstimator{
		estimator:        e,
		delayStatsWriter: dsw,
	}
}

func (e *slopeEstimator) onArrivalGroup(ag arrivalGroup) {
	if !e.init {
		e.group = ag
		e.init = true
		return
	}
	measurement := interGroupDelayVariation(e.group, ag)
	delta := ag.arrival.Sub(e.group.arrival)
	e.group = ag
	e.delayStatsWriter(DelayStats{
		Measurement:      measurement,
		Estimate:         e.updateEstimate(measurement, delta),
		Threshold:        0,
		LastReceiveDelta: delta,
		Usage:            0,
		State:            0,
		TargetBitrate:    0,
	})
}

func interGroupDelayVariation(a, b arrivalGroup) time.Duration {
	return b.arrival.Sub(a.arrival) - b.departure.Sub(a.departure)
}
