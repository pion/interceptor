package pacing

import (
	"math"
	"sync"
	"time"
)

type LeakyBucket struct {
	lock       sync.Mutex
	burstSize  int
	rate       int
	lastSent   time.Time
	lastBudget int
}

func NewLeakyBucketPacer(burstSize int, rate int) *LeakyBucket {
	return &LeakyBucket{
		lock:       sync.Mutex{},
		burstSize:  burstSize,
		rate:       0,
		lastSent:   time.Time{},
		lastBudget: 0,
	}
}

func (b *LeakyBucket) setRate(rate int) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.rate = rate
}

func (b *LeakyBucket) onSent(t time.Time, size int) {
	budget := b.budget(t)

	b.lock.Lock()
	defer b.lock.Unlock()

	if size > budget {
		b.lastBudget = 0
	} else {
		b.lastBudget = budget - size
	}
	b.lastSent = t
}

func (b *LeakyBucket) budget(t time.Time) int {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.lastSent.IsZero() {
		return b.burstSize
	}
	td := t.Sub(b.lastSent)
	budget := b.lastBudget + 8*int(float64(b.rate)*td.Seconds())
	if budget < 0 {
		budget = math.MaxInt
	}
	budget = min(budget, b.burstSize)
	return budget
}
