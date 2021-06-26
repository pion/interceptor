package gcc

import (
	"testing"
	"time"

	"github.com/pion/interceptor/internal/cc"
	"github.com/stretchr/testify/assert"
)

func TestRTTEstimator(t *testing.T) {
	cases := []struct {
		name     string
		ackLists [][]cc.Acknowledgment
		expected []time.Duration
	}{
		{
			name:     "noACKsNoRTT",
			ackLists: [][]cc.Acknowledgment{},
			expected: []time.Duration{},
		},
		{
			name: "staticRTT",
			ackLists: [][]cc.Acknowledgment{
				{{RTT: 5 * time.Millisecond}},
				{{RTT: 5 * time.Millisecond}},
				{{RTT: 5 * time.Millisecond}},
				{{RTT: 5 * time.Millisecond}},
			},
			expected: []time.Duration{
				5 * time.Millisecond,
				5 * time.Millisecond,
				5 * time.Millisecond,
				5 * time.Millisecond,
			},
		},
		{
			name: "",
			ackLists: [][]cc.Acknowledgment{
				{},
				{{RTT: 5 * time.Millisecond}},
				{{RTT: 5 * time.Millisecond}},
				{{RTT: 5 * time.Millisecond}},
				{{RTT: 5 * time.Millisecond}},
			},
			expected: []time.Duration{
				5 * time.Millisecond,
				5 * time.Millisecond,
				5 * time.Millisecond,
				5 * time.Millisecond,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			re := newRTTEstimator()
			in := make(chan []cc.Acknowledgment)
			out := re.run(in)
			go func() {
				for _, acks := range tc.ackLists {
					in <- acks
				}
				close(in)
			}()
			received := []time.Duration{}
			for rtt := range out {
				received = append(received, rtt)
			}
			assert.Equal(t, tc.expected, received)
		})
	}
}
