package gcc

const (
	qI = 1e-3
)

type kalman struct {
	gain                   float64
	estimate               float64
	estimateUncertainty    float64
	measurementUncertainty float64
}

func newKalman() *kalman {
	return &kalman{
		gain:                   0,
		estimate:               0,
		estimateUncertainty:    0.1 + qI,
		measurementUncertainty: 0.01,
	}
}

func (k *kalman) updateEstimate(measurement float64) float64 {
	k.gain = (k.estimateUncertainty) / (k.estimateUncertainty + k.measurementUncertainty)

	// TODO:
	// k.measurementUncertainty = alpha * var_v_hat(i-1) + (1-alpha) * z(i)^2

	k.estimate += k.gain * (measurement - k.estimate)

	k.estimateUncertainty = (1-k.gain)*k.estimateUncertainty + qI

	return k.estimate
}
