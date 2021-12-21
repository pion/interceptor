package gcc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateCalculator(t *testing.T) {
	rc := rateCalculator{
		history: []Acknowledgment{},
		window:  500 * time.Millisecond,
	}

	t0 := time.Now()

	rc.update([]Acknowledgment{})
	assert.Equal(t, 0, rc.rate)

	rc.update([]Acknowledgment{{
		TLCC:      0,
		Size:      125,
		Departure: time.Time{},
		Arrival:   t0.Add(-200 * time.Millisecond),
		RTT:       0,
	}})
	assert.Equal(t, 2000, rc.rate)

	rc.update([]Acknowledgment{{
		TLCC:      1,
		Size:      125,
		Departure: time.Time{},
		Arrival:   t0.Add(-100 * time.Millisecond),
		RTT:       0,
	}})
	assert.Equal(t, 4000, rc.rate)

	rc.update([]Acknowledgment{{
		TLCC:      0,
		Size:      125,
		Departure: time.Time{},
		Arrival:   t0.Add(350 * time.Millisecond),
		RTT:       0,
	}})
	assert.Equal(t, 4000, rc.rate)

	rc.update([]Acknowledgment{{
		TLCC:      0,
		Size:      0,
		Departure: time.Time{},
		Arrival:   t0.Add(time.Second),
		RTT:       0,
	}})
	assert.Equal(t, 0, rc.rate)
}
