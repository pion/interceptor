// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"math"
	"time"
)

const (
	chi = 0.001
)

type kalmanOption func(*kalman)

type kalman struct {
	gain                   float64
	estimate               time.Duration
	processUncertainty     float64 // Q_i
	estimateError          float64
	measurementUncertainty float64

	disableMeasurementUncertaintyUpdates bool
}

func initEstimate(e time.Duration) kalmanOption {
	return func(k *kalman) {
		k.estimate = e
	}
}

func initProcessUncertainty(p float64) kalmanOption {
	return func(k *kalman) {
		k.processUncertainty = p
	}
}

func initEstimateError(e float64) kalmanOption {
	return func(k *kalman) {
		k.estimateError = e * e // Only need variance from now on
	}
}

func initMeasurementUncertainty(u float64) kalmanOption {
	return func(k *kalman) {
		k.measurementUncertainty = u
	}
}

func setDisableMeasurementUncertaintyUpdates(b bool) kalmanOption {
	return func(k *kalman) {
		k.disableMeasurementUncertaintyUpdates = b
	}
}

func newKalman(opts ...kalmanOption) *kalman {
	k := &kalman{
		gain:                                 0,
		estimate:                             0,
		processUncertainty:                   1e-3,
		estimateError:                        0.1,
		measurementUncertainty:               0,
		disableMeasurementUncertaintyUpdates: false,
	}
	for _, opt := range opts {
		opt(k)
	}
	return k
}

func (k *kalman) updateEstimate(measurement time.Duration) time.Duration {
	z := measurement - k.estimate

	zms := float64(z.Microseconds()) / 1000.0

	if !k.disableMeasurementUncertaintyUpdates {
		alpha := math.Pow((1 - chi), 30.0/(1000.0*5*float64(time.Millisecond)))
		root := math.Sqrt(k.measurementUncertainty)
		root3 := 3 * root
		if zms > root3 {
			k.measurementUncertainty = math.Max(alpha*k.measurementUncertainty+(1-alpha)*root3*root3, 1)
		}
		k.measurementUncertainty = math.Max(alpha*k.measurementUncertainty+(1-alpha)*zms*zms, 1)
	}

	estimateUncertainty := k.estimateError + k.processUncertainty
	k.gain = estimateUncertainty / (estimateUncertainty + k.measurementUncertainty)

	k.estimate += time.Duration(k.gain * zms * float64(time.Millisecond))

	k.estimateError = (1 - k.gain) * estimateUncertainty
	return k.estimate
}
