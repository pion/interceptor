package gcc

const (
	chi = 0.1 // TODO: Tune between [0.1, 0.001]
	q_i = 1e-3
)

type kalman struct {
	gain                   float64
	estimate               float64
	estimateUncertainty    float64
	measurementUncertainty float64
}

func (k *kalman) updateEstimate1(measurement, fMax float64) float64 {
	k.estimate = k.estimate + 0.55*(measurement-k.estimate)
	return k.estimate
}

func newKalman() *kalman {
	return &kalman{
		gain:                   0,
		estimate:               0,
		estimateUncertainty:    0.1 + q_i,
		measurementUncertainty: 0.01,
	}
}

func (k *kalman) updateEstimate(measurement float64) float64 {
	k.gain = (k.estimateUncertainty) / (k.estimateUncertainty + k.measurementUncertainty)

	// TODO:
	//k.measurementUncertainty = alpha * var_v_hat(i-1) + (1-alpha) * z(i)^2

	k.estimate = k.estimate + k.gain*(measurement-k.estimate)

	k.estimateUncertainty = (1-k.gain)*k.estimateUncertainty + q_i

	return k.estimate
}
