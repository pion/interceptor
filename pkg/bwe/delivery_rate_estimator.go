package bwe

import (
	"container/heap"
	"time"
)

type deliveryRateHeapItem struct {
	arrival time.Time
	size    int
}

type deliveryRateHeap []deliveryRateHeapItem

// Len implements heap.Interface.
func (d deliveryRateHeap) Len() int {
	return len(d)
}

// Less implements heap.Interface.
func (d deliveryRateHeap) Less(i int, j int) bool {
	return d[i].arrival.Before(d[j].arrival)
}

// Pop implements heap.Interface.
func (d *deliveryRateHeap) Pop() any {
	old := *d
	n := len(old)
	x := old[n-1]
	*d = old[0 : n-1]
	return x
}

// Push implements heap.Interface.
func (d *deliveryRateHeap) Push(x any) {
	*d = append(*d, x.(deliveryRateHeapItem))
}

// Swap implements heap.Interface.
func (d deliveryRateHeap) Swap(i int, j int) {
	d[i], d[j] = d[j], d[i]
}

type deliveryRateEstimator struct {
	window        time.Duration
	latestArrival time.Time
	history       *deliveryRateHeap
}

func newDeliveryRateEstimator(window time.Duration) *deliveryRateEstimator {
	return &deliveryRateEstimator{
		window:        window,
		latestArrival: time.Time{},
		history:       &deliveryRateHeap{},
	}
}

func (e *deliveryRateEstimator) OnPacketAcked(arrival time.Time, size int) {
	if arrival.After(e.latestArrival) {
		e.latestArrival = arrival
	}
	heap.Push(e.history, deliveryRateHeapItem{
		arrival: arrival,
		size:    size,
	})
}

func (e *deliveryRateEstimator) GetRate() int {
	deadline := e.latestArrival.Add(-e.window)
	for len(*e.history) > 0 && (*e.history)[0].arrival.Before(deadline) {
		heap.Pop(e.history)
	}
	earliest := e.latestArrival
	sum := 0
	for _, i := range *e.history {
		if i.arrival.Before(earliest) {
			earliest = i.arrival
		}
		sum += i.size
	}
	d := e.latestArrival.Sub(earliest)
	if d == 0 {
		return 0
	}
	rate := 8 * float64(sum) / d.Seconds()
	return int(rate)
}
