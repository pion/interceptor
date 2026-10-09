// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"bufio"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/interceptor/internal/cc"
	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// End-to-end tests of the delay controller: packet traces in, DelayStats
// out. Traces are replayed in real time (feedback every 50 ms), so wall
// clock based logic sees the same pacing it would in production.

const (
	replayInitialBitrate = 1_200_000
	replayMinBitrate     = 150_000
	replayFeedback       = 50 * time.Millisecond
)

// tracePacket is one sent packet: departure on the sender clock, arrival on
// the receiver clock, both relative to their trace origin.
type tracePacket struct {
	size      int
	departure time.Duration
	arrival   time.Duration
}

// replayTrace feeds the trace to a fresh delay controller, one feedback
// report per replayFeedback of arrival time, and returns every DelayStats
// it produced.
func replayTrace(t *testing.T, trace []tracePacket) []DelayStats {
	t.Helper()
	sort.SliceStable(trace, func(i, j int) bool { return trace[i].departure < trace[j].departure })

	var (
		mu    sync.Mutex
		stats []DelayStats
	)
	dc := newDelayController(delayControllerConfig{
		nowFn:          time.Now,
		initialBitrate: replayInitialBitrate,
		minBitrate:     replayMinBitrate,
		maxBitrate:     50_000_000,
	}, logging.NewDefaultLoggerFactory())
	dc.onUpdate(func(ds DelayStats) {
		mu.Lock()
		defer mu.Unlock()
		stats = append(stats, ds)
	})

	sendOrigin := time.Unix(1_700_000_000, 0)
	recvOrigin := sendOrigin.Add(-37 * time.Second) // the receiver clock is unrelated
	acks := make([]cc.Acknowledgment, len(trace))
	firstArrival, lastArrival := trace[0].arrival, trace[0].arrival
	for i, p := range trace {
		acks[i] = cc.Acknowledgment{
			SequenceNumber: uint16(i), //nolint:gosec // wraps like TWCC does
			Size:           p.size,
			Departure:      sendOrigin.Add(p.departure),
			Arrival:        recvOrigin.Add(p.arrival),
		}
		firstArrival = min(firstArrival, p.arrival)
		lastArrival = max(lastArrival, p.arrival)
	}

	start := time.Now()
	for window := firstArrival; window <= lastArrival; window += replayFeedback {
		var report []cc.Acknowledgment
		for i, p := range trace {
			if p.arrival >= window && p.arrival < window+replayFeedback {
				report = append(report, acks[i])
			}
		}
		time.Sleep(time.Until(start.Add(window - firstArrival + replayFeedback)))
		if len(report) > 0 {
			dc.updateDelayEstimate(report)
		}
	}
	require.NoError(t, dc.Close())

	mu.Lock()
	defer mu.Unlock()

	return stats
}

func decreases(stats []DelayStats) int {
	n := 0
	for _, ds := range stats {
		if ds.State == stateDecrease {
			n++
		}
	}

	return n
}

// summarize logs what a replay did: samples, decreases, the lowest and
// the final target.
func summarize(t *testing.T, stats []DelayStats) {
	t.Helper()
	lowest := 0
	for _, ds := range stats {
		if lowest == 0 || ds.TargetBitrate < lowest {
			lowest = ds.TargetBitrate
		}
	}
	final := 0
	if len(stats) > 0 {
		final = stats[len(stats)-1].TargetBitrate
	}
	t.Logf("%d samples, %d decreases, lowest target %d, final target %d", len(stats), decreases(stats), lowest, final)
}

// loadGroupTrace reads arrival groups (n,bytes,depFirst,depLast,arrFirst,
// arrLast in ms) and spreads each group's packets evenly over its span.
func loadGroupTrace(t *testing.T, path string) []tracePacket {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // test data
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	ms := func(s string) time.Duration {
		v, perr := strconv.ParseFloat(s, 64)
		require.NoError(t, perr)

		return time.Duration(v * float64(time.Millisecond))
	}
	var trace []tracePacket
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ",")
		require.Len(t, fields, 6, line)
		n, err := strconv.Atoi(fields[0])
		require.NoError(t, err)
		bytes, err := strconv.Atoi(fields[1])
		require.NoError(t, err)
		depFirst, depLast := ms(fields[2]), ms(fields[3])
		arrFirst, arrLast := ms(fields[4]), ms(fields[5])
		for i := range n {
			frac := 0.0
			if n > 1 {
				frac = float64(i) / float64(n-1)
			}
			trace = append(trace, tracePacket{
				size:      bytes / n,
				departure: depFirst + time.Duration(frac*float64(depLast-depFirst)),
				arrival:   arrFirst + time.Duration(frac*float64(arrLast-arrFirst)),
			})
		}
	}
	require.NoError(t, scanner.Err())

	return trace
}

// jitter is a deterministic ±250 µs perturbation.
type jitter uint32

func (j *jitter) next() time.Duration {
	*j = *j*1664525 + 1013904223

	return time.Duration(int64(*j>>16)%500-250) * time.Microsecond
}

// audioThenVideo is a call as an SFU forwards it: Opus audio (120 B every
// 20 ms) from t=0 and, from videoStart on, 15 fps video whose frame sizes
// vary like VP9 SVC pictures (a 10-packet keyframe, then 2–7 packets,
// slowly growing as layers are added). A 5 ms pacer releases up to perTick
// packets per tick, 50 µs apart. link maps a departure and size to an
// arrival.
func audioThenVideo(
	videoStart, end time.Duration, perTick int, link func(dep time.Duration, size int) time.Duration,
) []tracePacket {
	var trace []tracePacket
	for dep := time.Duration(0); dep < end; dep += 20 * time.Millisecond {
		trace = append(trace, tracePacket{size: 120, departure: dep})
	}
	var sizes jitter = 11
	frame := 0
	for dep := videoStart; dep < end; dep += time.Second / 15 {
		packets := 10
		if frame > 0 {
			packets = 2 + int(uint32(sizes)>>8)%(3+min(frame/10, 3))
			sizes.next()
		}
		for i := 0; i < packets; i++ {
			tick, slot := i/perTick, i%perTick
			trace = append(trace, tracePacket{
				size:      1100,
				departure: dep + time.Duration(tick)*5*time.Millisecond + time.Duration(slot)*50*time.Microsecond,
			})
		}
		frame++
	}
	sort.SliceStable(trace, func(i, j int) bool { return trace[i].departure < trace[j].departure })
	for i := range trace {
		trace[i].arrival = link(trace[i].departure, trace[i].size)
	}

	return trace
}

// bottleneck is a FIFO link of the given capacity behind 10 ms of
// propagation delay.
func bottleneck(bitsPerSecond int) func(time.Duration, int) time.Duration {
	var free time.Duration

	return func(dep time.Duration, size int) time.Duration {
		free = max(free, dep+10*time.Millisecond) +
			time.Duration(int64(size)*8*int64(time.Second)/int64(bitsPerSecond))

		return free
	}
}

// A fresh subscriber's real join (CI run, loopback, no loss): the start of
// video is not congestion. v0.1.45 cut the target to the floor here.
func TestDelayControllerJoinReplayIsNotCongestion(t *testing.T) {
	t.Parallel()
	stats := replayTrace(t, loadGroupTrace(t, "testdata/join_audio_then_video.csv"))
	summarize(t, stats)

	require.Greater(t, len(stats), 150, "replay produced too few delay samples to judge")
	assert.Zero(t, decreases(stats), "overuse signaled on an uncongested join")
	assert.GreaterOrEqual(t, stats[len(stats)-1].TargetBitrate, replayInitialBitrate)
}

// Paced video frames of varying size on an uncongested link (100 Mbps, ±250 µs
// jitter) must not read as queueing delay.
func TestDelayControllerPacedVideoOnCleanLink(t *testing.T) {
	t.Parallel()
	var j jitter = 7
	clean := func(dep time.Duration, size int) time.Duration {
		return dep + 10*time.Millisecond + time.Duration(size*80)*time.Nanosecond + j.next()
	}
	stats := replayTrace(t, audioThenVideo(2*time.Second, 5*time.Second, 3, clean))
	summarize(t, stats)

	require.Greater(t, len(stats), 150, "replay produced too few delay samples to judge")
	assert.Zero(t, decreases(stats), "overuse signaled on a clean link")
}

// Unhappy path: a real bottleneck (video ~700 kbps into 400 kbps) must
// still be detected and the target cut.
func TestDelayControllerDetectsBottleneck(t *testing.T) {
	t.Parallel()
	stats := replayTrace(t, audioThenVideo(time.Second, 4*time.Second, 3, bottleneck(400_000)))
	summarize(t, stats)

	require.NotEmpty(t, stats)
	assert.Positive(t, decreases(stats), "a filling queue was not detected")
	assert.Less(t, stats[len(stats)-1].TargetBitrate, replayInitialBitrate)
}

// Unhappy path, mild: ~10% above capacity still builds a queue that the
// detector must see before the trace ends.
func TestDelayControllerDetectsMildBottleneck(t *testing.T) {
	t.Parallel()
	// The video averages ~5 × 1100 B × 15 fps ≈ 660 kbps (+ 48 kbps audio).
	stats := replayTrace(t, audioThenVideo(time.Second, 6*time.Second, 3, bottleneck(640_000)))
	summarize(t, stats)

	require.NotEmpty(t, stats)
	assert.Positive(t, decreases(stats), "a slowly filling queue was not detected")
}
