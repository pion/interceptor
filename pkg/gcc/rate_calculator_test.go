package gcc

import (
	"testing"
	"time"

	"github.com/pion/interceptor/internal/cc"
	"github.com/stretchr/testify/assert"
)

func TestRateCalculator(t *testing.T) {
	t0 := time.Now()
	cases := []struct {
		name     string
		acks     []cc.Acknowledgment
		expected int
	}{
		{
			name:     "emptyCreatesNoRate",
			acks:     []cc.Acknowledgment{},
			expected: 0,
		},
		{
			name: "ignoresZeroArrivalTimes",
			acks: []cc.Acknowledgment{{
				SequenceNumber: 0,
				Size:           0,
				Departure:      t0,
				Arrival:        t0.Add(10 * time.Millisecond),
			}},
			expected: 0,
		},
		{
			name: "lessThanRequiredAcksCreatesNoRate",
			acks: []cc.Acknowledgment{{
				SequenceNumber: 0,
				Size:           1000,
				Departure:      t0,
				Arrival:        t0.Add(10 * time.Millisecond),
			}},
			expected: 0,
		},
		{
			name:     "steadyACKsCalculateCorrectRates",
			acks:     getACKStream(100, 1200, 100*time.Millisecond),
			expected: 95049,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rc := newRateCalculator()
			in := make(chan []cc.Acknowledgment)
			out := make(chan int)
			onRateUpdate := func(rate int) {
				out <- rate
			}
			go func() {
				defer close(out)
				rc.run(in, onRateUpdate)
			}()
			go func() {
				in <- tc.acks
				close(in)
			}()

			received := []int{}
			for r := range out {
				received = append(received, r)
			}
			if tc.expected != 0 {
				assert.Equal(t, 1, len(received))
				assert.Equal(t, tc.expected, received[0])
			} else {
				assert.Equal(t, 0, len(received))
			}
		})
	}
}

func getACKStream(length int, size int, interval time.Duration) []cc.Acknowledgment {
	res := []cc.Acknowledgment{}
	t0 := time.Now()
	for i := 0; i < length; i++ {
		res = append(res, cc.Acknowledgment{
			Size:      size,
			Departure: t0.Add(time.Duration(i) * time.Millisecond),
			Arrival:   t0.Add(5*time.Millisecond + time.Duration(i)*time.Millisecond),
		})
		t0 = t0.Add(interval)
	}
	return res
}
