package gcc

type slopeEstimator struct {
	estimator
}

func newSlopeEstimator(e estimator) *slopeEstimator {
	return &slopeEstimator{
		estimator: e,
	}
}

func (e *slopeEstimator) run(in <-chan arrivalGroup) <-chan DelayStats {
	out := make(chan DelayStats)
	go func() {
		init := false
		var last arrivalGroup
		for next := range in {
			if !init {
				last = next
				init = true
				continue
			}
			measurement := interGroupDelayVariation(last, next)
			delta := next.arrival.Sub(last.arrival)
			last = next
			out <- DelayStats{
				Measurement:      measurement,
				Estimate:         e.updateEstimate(measurement),
				Threshold:        0,
				lastReceiveDelta: delta,
				Usage:            0,
				State:            0,
				TargetBitrate:    0,
				RTT:              0,
			}
		}
		close(out)
	}()
	return out
}
