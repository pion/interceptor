package bwe

type exponentialMovingAverage struct {
	initialized bool
	alpha       float64
	average     float64
	variance    float64
}

func (a *exponentialMovingAverage) update(sample float64) {
	if !a.initialized {
		a.average = sample
		a.initialized = true
	} else {
		delta := sample - a.average
		a.average = a.alpha*sample + (1-a.alpha)*a.average
		a.variance = (1-a.alpha)*a.variance + a.alpha*(1-a.alpha)*(delta*delta)
	}
}
