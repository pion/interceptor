package gcc

import (
	"time"

	"github.com/pion/interceptor/internal/cc"
)

type rateCalculator struct {
	window time.Duration
}

func newRateCalculator(window time.Duration) *rateCalculator {
	return &rateCalculator{
		window: window,
	}
}

func (c *rateCalculator) run(in <-chan cc.Acknowledgment) <-chan int {
	out := make(chan int)
	go func() {
		var history []cc.Acknowledgment
		init := false
		sum := 0
		for next := range in {
			if next.Arrival.IsZero() {
				// Ignore packet if it didn't arrive
				continue
			}
			history = append(history, next)
			sum += next.Size

			if !init {
				init = true
				// Don't know any timeframe here, only arrival of last packet,
				// which is by definition in the window that ends with the last
				// arrival time
				out <- next.Size * 8
				continue
			}

			del := 0
			for _, ack := range history {
				deadline := next.Arrival.Add(-c.window)
				if !ack.Arrival.Before(deadline) {
					break
				}
				del++
				sum -= ack.Size
			}
			history = history[del:]
			if len(history) == 0 {
				out <- 0
				continue
			}
			dt := next.Arrival.Sub(history[0].Arrival)
			bits := 8 * sum
			rate := int(float64(bits) / dt.Seconds())
			out <- rate
		}
		close(out)
	}()
	return out
}
