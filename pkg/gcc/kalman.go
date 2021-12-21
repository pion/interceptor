package gcc

import (
	"math"
)

const (
	qI    = 1e-3
	alpha = 0.95
)

type kalman struct {
	gain                   float64
	estimate               float64
	estimateUncertainty    float64
	measurementUncertainty float64
}

func newKalman() *kalman {
	return &kalman{
		gain:                   0.95,
		estimate:               0,
		estimateUncertainty:    0.1 + qI,
		measurementUncertainty: 0.1,
	}
}

func (k *kalman) updateEstimate(measurement float64) float64 {
	z := measurement - k.estimate

	k.measurementUncertainty = math.Max(alpha*k.measurementUncertainty+(1-alpha)*z*z, 1)

	root := math.Sqrt(k.measurementUncertainty)
	if z > 3*root {
		z = 3 * root
	}

	k.gain = (k.estimateUncertainty) / (k.estimateUncertainty + k.measurementUncertainty)

	k.estimate += k.gain * z

	k.estimateUncertainty = (1 - k.gain) * (k.estimateUncertainty + qI)

	return k.estimate
}
