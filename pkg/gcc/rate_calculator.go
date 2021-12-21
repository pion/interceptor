package gcc

import "time"

type rateCalculator struct {
	history []Acknowledgment
	window  time.Duration
	rate    int
}

func (rc *rateCalculator) update(acks []Acknowledgment) {
	rc.history = append(rc.history, acks...)
	sum := 0
	del := 0
	if len(rc.history) == 0 {
		rc.rate = 0
		return
	}
	now := rc.history[len(rc.history)-1].Arrival
	for _, ack := range rc.history {
		if now.Sub(ack.Arrival) > rc.window {
			del++
			continue
		}
		if !ack.Arrival.IsZero() {
			sum += ack.Size
		}
	}
	rc.history = rc.history[del:]
	rc.rate = int(float64(8*sum) / rc.window.Seconds())
}
