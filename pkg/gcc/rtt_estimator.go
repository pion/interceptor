package gcc

import (
	"math"
	"time"

	"github.com/pion/interceptor/internal/cc"
)

type rttEstimator struct {
	samples int
}

func newRTTEstimator() *rttEstimator {
	return &rttEstimator{
		samples: 100,
	}
}

func (e *rttEstimator) run(in <-chan []cc.Acknowledgment) <-chan time.Duration {
	out := make(chan time.Duration)
	go func() {
		history := []time.Duration{}
		for acks := range in {
			if len(acks) == 0 {
				continue
			}
			minRTT := time.Duration(math.MaxInt64)
			for _, ack := range acks {
				if ack.RTT < minRTT {
					minRTT = ack.RTT
				}
			}
			history = append(history, minRTT)
			if len(history) >= e.samples {
				history = history[len(history)-e.samples:]
			}

			sum := time.Duration(0)
			for _, rtt := range history {
				sum += rtt
			}
			rtt := float64(sum.Milliseconds()) / float64(len(history)) * float64(time.Millisecond)
			out <- time.Duration(rtt)
		}
		close(out)
	}()
	return out
}
