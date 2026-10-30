// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"time"

	"github.com/pion/interceptor/internal/cc"
)

// Matches libwebrtc's RobustThroughputEstimatorSettings::max_window_packets.
// https://webrtc.googlesource.com/src/+/refs/heads/main/modules/congestion_controller/goog_cc/acknowledged_bitrate_estimator_interface.h
const maxRateHistoryPackets = 500

// Matches libwebrtc's RobustThroughputEstimator::kMaxReorderingTime.
const maxRateReorderingTime = time.Second

type rateCalculator struct {
	window time.Duration
}

type rateSample struct {
	arrival time.Time
	size    int
}

type rateHistory struct {
	samples [maxRateHistoryPackets]rateSample
	start   int
	count   int
	sum     int
}

func (h *rateHistory) add(next cc.Acknowledgment, window time.Duration) bool {
	latest := next.Arrival
	if h.count > 0 {
		previous := h.latestArrival()
		if previous.Sub(next.Arrival) > maxRateReorderingTime {
			*h = rateHistory{}
		} else if previous.After(latest) {
			latest = previous
		}
	}
	deadline := latest.Add(-window)
	if next.Arrival.Before(deadline) {
		return false
	}
	h.trim(deadline)
	if h.count == len(h.samples) {
		if next.Arrival.Before(h.samples[h.start].arrival) {
			return false
		}
		h.removeOldest()
	}
	h.insert(rateSample{arrival: next.Arrival, size: next.Size})

	return true
}

func (h *rateHistory) latestArrival() time.Time {
	return h.samples[(h.start+h.count-1)%len(h.samples)].arrival
}

func (h *rateHistory) removeOldest() {
	h.sum -= h.samples[h.start].size
	h.samples[h.start] = rateSample{}
	h.start = (h.start + 1) % len(h.samples)
	h.count--
}

func (h *rateHistory) trim(deadline time.Time) {
	for h.count > 0 && h.samples[h.start].arrival.Before(deadline) {
		h.removeOldest()
	}
}

func (h *rateHistory) insert(sample rateSample) {
	position := h.count
	for position > 0 {
		previous := h.samples[(h.start+position-1)%len(h.samples)]
		if !sample.arrival.Before(previous.arrival) {
			break
		}
		h.samples[(h.start+position)%len(h.samples)] = previous
		position--
	}
	h.samples[(h.start+position)%len(h.samples)] = sample
	h.count++
	h.sum += sample.size
}

func newRateCalculator(window time.Duration) *rateCalculator {
	return &rateCalculator{
		window: window,
	}
}

func (c *rateCalculator) run(in <-chan []cc.Acknowledgment, onRateUpdate func(int)) {
	history := rateHistory{}
	init := false
	for acks := range in {
		for _, next := range acks {
			if next.Arrival.IsZero() {
				// Ignore packet if it didn't arrive
				continue
			}
			if !history.add(next, c.window) {
				continue
			}

			if !init {
				init = true
				// Don't know any timeframe here, only arrival of last packet,
				// which is by definition in the window that ends with the last
				// arrival time
				onRateUpdate(next.Size * 8)

				continue
			}

			dt := history.latestArrival().Sub(history.samples[history.start].arrival)
			if dt <= 0 {
				continue
			}
			bits := 8 * history.sum
			rate := int(float64(bits) / dt.Seconds())
			onRateUpdate(rate)
		}
	}
}
