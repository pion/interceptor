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
	if h.count > 0 && next.Arrival.Before(h.samples[(h.start+h.count-1)%len(h.samples)].arrival) {
		return false
	}
	deadline := next.Arrival.Add(-window)
	for h.count > 0 && (h.count == len(h.samples) || h.samples[h.start].arrival.Before(deadline)) {
		h.sum -= h.samples[h.start].size
		h.samples[h.start] = rateSample{}
		h.start = (h.start + 1) % len(h.samples)
		h.count--
	}
	h.samples[(h.start+h.count)%len(h.samples)] = rateSample{arrival: next.Arrival, size: next.Size}
	h.count++
	h.sum += next.Size

	return true
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

			dt := next.Arrival.Sub(history.samples[history.start].arrival)
			if dt <= 0 {
				continue
			}
			bits := 8 * history.sum
			rate := int(float64(bits) / dt.Seconds())
			onRateUpdate(rate)
		}
	}
}
