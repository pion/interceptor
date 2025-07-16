package bwe

import (
	"math"
)

type kalmanFilter struct {
	state [2]float64 // [slope, offset]

	processNoise [2]float64
	e            [2][2]float64
	avgNoise     float64
	varNoise     float64
}

type kalmanOption func(*kalmanFilter)

func initSlope(e float64) kalmanOption {
	return func(k *kalmanFilter) {
		k.state[0] = e
	}
}

func newKalmanFilter(opts ...kalmanOption) *kalmanFilter {
	kf := &kalmanFilter{
		state:        [2]float64{8.0 / 512.0, 0},
		processNoise: [2]float64{1e-13, 1e-3},
		e:            [2][2]float64{{100.0, 0}, {0, 1e-1}},
		varNoise:     50.0,
	}
	for _, opt := range opts {
		opt(kf)
	}
	return kf
}

func (k *kalmanFilter) update(timeDelta float64, sizeDelta float64) float64 {
	k.e[0][0] += k.processNoise[0]
	k.e[1][1] += k.processNoise[1]

	h := [2]float64{sizeDelta, 1.0}
	Eh := [2]float64{
		k.e[0][0]*h[0] + k.e[0][1]*h[1],
		k.e[1][0]*h[0] + k.e[1][1]*h[1],
	}
	residual := timeDelta - (k.state[0]*h[0] + k.state[1]*h[1])

	maxResidual := 3.0 * math.Sqrt(k.varNoise)
	if math.Abs(residual) < maxResidual {
		k.updateNoiseEstimate(residual, timeDelta)
	} else {
		if residual < 0 {
			k.updateNoiseEstimate(-maxResidual, timeDelta)
		} else {
			k.updateNoiseEstimate(maxResidual, timeDelta)
		}
	}

	denom := k.varNoise + h[0]*Eh[0] + h[1]*Eh[1]

	K := [2]float64{
		Eh[0] / denom, Eh[1] / denom,
	}

	IKh := [2][2]float64{
		{1.0 - K[0]*h[0], -K[0] * h[1]},
		{-K[1] * h[0], 1.0 - K[1]*h[1]},
	}

	e00 := k.e[0][0]
	e01 := k.e[0][1]

	k.e[0][0] = e00*IKh[0][0] + k.e[1][0]*IKh[0][1]
	k.e[0][1] = e01*IKh[0][0] + k.e[1][1]*IKh[0][1]
	k.e[1][0] = e00*IKh[1][0] + k.e[1][0]*IKh[1][1]
	k.e[1][1] = e01*IKh[1][0] + k.e[1][1]*IKh[1][1]

	k.state[0] = k.state[0] + K[0]*residual
	k.state[1] = k.state[1] + K[1]*residual

	return k.state[1]
}

func (k *kalmanFilter) updateNoiseEstimate(residual float64, timeDelta float64) {
	alpha := 0.002
	beta := math.Pow(1-alpha, timeDelta*30.0/1000.0)
	k.avgNoise = beta*k.avgNoise + (1-beta)*residual
	k.varNoise = beta*k.varNoise + (1-beta)*(k.avgNoise-residual)*(k.avgNoise-residual)
	if k.varNoise < 1 {
		k.varNoise = 1
	}
}
