// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main demonstrates a dynamic jitter buffer fed over a WebRTC data
// channel by a browser page that simulates latency, jitter, bandwidth and loss.
package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pion/interceptor/pkg/jitterbuffer"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

const (
	clockRate      = 90000
	playoutTick    = 2 * time.Millisecond
	statusInterval = time.Second
	rateWindow     = 2 // seconds of media
	barWidth       = 20

	// Catch-up drops backlog that every packet played over catchUpWindow
	// waited at least catchUpMargin longer than needed for.
	catchUpWindow = 2 * time.Second
	catchUpMargin = 20 * time.Millisecond
	arrivalSlots  = 1024
)

//go:embed index.html
var indexHTML []byte

func main() {
	addr := flag.String("addr", "localhost:8080", "HTTP listen address")
	targetDelay := flag.Duration("target-delay", 100*time.Millisecond, "playout delay the buffer is sized to cover")
	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(indexHTML)
	})
	http.HandleFunc("/offer", func(w http.ResponseWriter, r *http.Request) {
		answer, err := answerOffer(r, *targetDelay)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}
		_ = json.NewEncoder(w).Encode(answer)
	})

	log.Printf("open http://%s (target delay %s)", *addr, *targetDelay)
	server := &http.Server{Addr: *addr, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServe())
}

func answerOffer(r *http.Request, targetDelay time.Duration) (*webrtc.SessionDescription, error) {
	var offer webrtc.SessionDescription
	if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
		return nil, err
	}

	peer, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, err
	}
	peer.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("peer connection %s", state)
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateDisconnected {
			_ = peer.Close()
		}
	})
	peer.OnDataChannel(func(channel *webrtc.DataChannel) {
		newReceiver(targetDelay).attach(channel)
	})

	if err = peer.SetRemoteDescription(offer); err != nil {
		return nil, err
	}
	answer, err := peer.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}
	gathered := webrtc.GatheringCompletePromise(peer)
	if err = peer.SetLocalDescription(answer); err != nil {
		return nil, err
	}
	<-gathered

	return peer.LocalDescription(), nil
}

// mediaRate estimates the stream bitrate from the bytes received over a
// trailing window of RTP timestamps. It stands in for a congestion-control
// estimator such as GCC, which needs RTP transport feedback a data channel
// does not carry. Measuring against media time rather than arrival time keeps
// stalls and jitter from distorting the estimate.
type mediaRate struct {
	mu      sync.Mutex
	samples []rateSample
}

type rateSample struct {
	timestamp uint32
	bytes     int
}

func (m *mediaRate) add(timestamp uint32, bytes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples = append(m.samples, rateSample{timestamp, bytes})
	expired := 0
	for expired < len(m.samples) && tsDiff(timestamp, m.samples[expired].timestamp) > rateWindow*clockRate {
		expired++
	}
	m.samples = m.samples[expired:]
}

// GetTargetBitrate returns the stream's bits per second, or 0 until a tenth of
// a second of media has been received.
func (m *mediaRate) GetTargetBitrate() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.samples) < 2 {
		return 0
	}
	span := tsDiff(m.samples[len(m.samples)-1].timestamp, m.samples[0].timestamp)
	if span < clockRate/10 {
		return 0
	}
	total := 0
	for _, sample := range m.samples[1:] {
		total += sample.bytes
	}

	return int(int64(total) * 8 * clockRate / int64(span))
}

// receiver plays packets out of a dynamic jitter buffer in real time, paced by
// their RTP timestamps.
type receiver struct {
	mu        sync.Mutex
	buffer    *jitterbuffer.JitterBuffer
	estimator *mediaRate

	// The playout clock plays RTP timestamp base at start.
	anchored bool
	base     uint32
	start    time.Time
	pending  *rtp.Packet
	played   int
	lastSeq  uint16
	lastTS   uint32
	step     uint32

	// arrivals records when each recent packet was received, so catch-up can
	// tell how long packets waited in the buffer before being played.
	arrivals    [arrivalSlots]arrival
	windowStart time.Time
	windowLate  int
	minWait     time.Duration

	received          bool
	newestSeq         uint16
	newestTS          uint32
	inCount, outCount int
	late              int
	lastTarget        uint16
}

type arrival struct {
	seq uint16
	at  time.Time
}

func newReceiver(targetDelay time.Duration) *receiver {
	recv := &receiver{estimator: &mediaRate{}}
	recv.buffer = jitterbuffer.New(
		jitterbuffer.WithCopy(),
		jitterbuffer.WithBandwidthEstimator(recv.estimator),
		jitterbuffer.WithDynamicLength(jitterbuffer.DynamicLengthConfig{TargetDelay: targetDelay}),
	)
	recv.lastTarget = recv.buffer.TargetLength()
	recv.buffer.Listen(jitterbuffer.BeginPlayback, func(_ jitterbuffer.Event, jb *jitterbuffer.JitterBuffer) {
		log.Printf("event: playback started with %d packets buffered", jb.Length())
	})
	recv.buffer.Listen(jitterbuffer.BufferUnderflow, func(_ jitterbuffer.Event, jb *jitterbuffer.JitterBuffer) {
		log.Printf("event: underflow #%d, rebuffering to %d packets", jb.Stats().UnderflowCount(), jb.TargetLength())
	})
	recv.buffer.Listen(jitterbuffer.BufferOverflow, func(jitterbuffer.Event, *jitterbuffer.JitterBuffer) {
		log.Printf("event: overflow, packet dropped")
	})

	return recv
}

func (r *receiver) attach(channel *webrtc.DataChannel) {
	done := make(chan struct{})
	channel.OnOpen(func() {
		log.Printf("data channel %q open", channel.Label())
		go r.run(channel, done)
	})
	channel.OnClose(func() {
		log.Printf("data channel %q closed", channel.Label())
		close(done)
	})
	channel.OnMessage(func(msg webrtc.DataChannelMessage) {
		packet := &rtp.Packet{}
		if err := packet.Unmarshal(msg.Data); err != nil {
			log.Printf("dropping malformed packet: %v", err)

			return
		}
		r.estimator.add(packet.Timestamp, len(msg.Data))
		r.mu.Lock()
		defer r.mu.Unlock()
		if !r.received || seqDiff(packet.SequenceNumber, r.newestSeq) > 0 {
			r.received, r.newestSeq, r.newestTS = true, packet.SequenceNumber, packet.Timestamp
		}
		r.inCount++
		if r.played > 0 && seqDiff(packet.SequenceNumber, r.lastSeq) <= 0 {
			r.late++
		}
		r.arrivals[packet.SequenceNumber%arrivalSlots] = arrival{packet.SequenceNumber, time.Now()}
		r.buffer.Push(packet)
	})
}

func (r *receiver) run(channel *webrtc.DataChannel, done <-chan struct{}) {
	playout := time.NewTicker(playoutTick)
	defer playout.Stop()
	status := time.NewTicker(statusInterval)
	defer status.Stop()
	for {
		select {
		case <-done:
			return
		case now := <-playout.C:
			r.playout(now)
		case <-status.C:
			line := r.status()
			log.Print(line)
			_ = channel.SendText(line)
		}
	}
}

func (r *receiver) playout(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.buffer.State() != jitterbuffer.Emitting {
		r.anchored = false
		r.windowStart = time.Time{}

		return
	}
	r.catchUp(now)
	for packet := r.next(now); packet != nil; packet = r.next(now) {
		if !r.anchored {
			r.anchored, r.base, r.start = true, packet.Timestamp, now
		}
		if !r.due(packet.Timestamp, now) {
			return
		}
		r.pending = nil
		r.play(packet, now)
	}
	if r.buffer.State() != jitterbuffer.Emitting {
		r.anchored = false
	}
}

// next returns the packet to play next, or nil if none is available.
func (r *receiver) next(now time.Time) *rtp.Packet {
	// Without an anchored clock the next packet is due immediately and anchors it.
	nextDue := !r.anchored || (r.step != 0 && r.due(r.lastTS+r.step, now))
	if r.pending == nil && (r.buffer.Length() > 0 || nextDue) {
		// With nothing queued, Pop either skips past a lost packet or
		// underflows and starts rebuffering.
		r.pending, _ = r.buffer.Pop()
	}

	return r.pending
}

// catchUp drops the oldest queued packets when every packet played over the
// last catchUpWindow waited longer than needed, as after the burst that
// follows a network stall, so latency does not stay raised for the rest of
// the session. A packet arriving after its turn was skipped shows the delay
// is too short, not too long, so nothing is dropped in that window.
func (r *receiver) catchUp(now time.Time) {
	if r.windowStart.IsZero() || r.late != r.windowLate {
		r.resetWindow(now)

		return
	}
	if now.Sub(r.windowStart) < catchUpWindow {
		return
	}
	excess := r.minWait - catchUpMargin
	r.resetWindow(now)
	if r.played > 0 && excess > 0 {
		r.dropBacklog(excess)
	}
}

// dropBacklog drops up to excess of the oldest media and moves the playout
// clock forward to match.
func (r *receiver) dropBacklog(excess time.Duration) {
	firstTS, dropped := r.lastTS, 0
	// Popping past the newest received packet would underflow.
	for r.pending != nil || seqDiff(r.newestSeq, r.lastSeq) > 0 {
		packet := r.pending
		if packet == nil {
			packet, _ = r.buffer.Pop()
			r.pending = packet
		}
		if packet == nil || time.Duration(tsDiff(packet.Timestamp, firstTS))*time.Second/clockRate > excess {
			break
		}
		r.pending = nil
		r.lastSeq, r.lastTS = packet.SequenceNumber, packet.Timestamp
		dropped++
	}
	if dropped == 0 {
		return
	}
	r.start = r.start.Add(-time.Duration(tsDiff(r.lastTS, firstTS)) * time.Second / clockRate)
	log.Printf("event: dropped %d packets: for %s every packet waited at least %dms longer than needed",
		dropped, catchUpWindow, excess.Milliseconds())
}

func (r *receiver) resetWindow(now time.Time) {
	r.windowStart, r.windowLate, r.minWait = now, r.late, time.Duration(1<<63-1)
}

// buffered is the media time between the last played packet and the newest received.
func (r *receiver) buffered() time.Duration {
	return time.Duration(tsDiff(r.newestTS, r.lastTS)) * time.Second / clockRate
}

func (r *receiver) due(timestamp uint32, now time.Time) bool {
	elapsed := uint32(now.Sub(r.start).Seconds() * clockRate)

	return tsDiff(timestamp-r.base, elapsed) <= 0
}

func (r *receiver) play(packet *rtp.Packet, now time.Time) {
	if a := r.arrivals[packet.SequenceNumber%arrivalSlots]; a.seq == packet.SequenceNumber {
		r.minWait = min(r.minWait, now.Sub(a.at))
	}
	gap, elapsed := packet.SequenceNumber-r.lastSeq, tsDiff(packet.Timestamp, r.lastTS)
	if r.played > 0 && gap > 0 && gap < 1<<15 && elapsed > 0 {
		r.step = uint32(elapsed) / uint32(gap)
	}
	r.lastSeq, r.lastTS = packet.SequenceNumber, packet.Timestamp
	r.played++
	r.outCount++
}

func (r *receiver) status() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	target := r.buffer.TargetLength()
	if target != r.lastTarget {
		log.Printf("event: target length %d -> %d packets", r.lastTarget, target)
		r.lastTarget = target
	}
	// The span counts packets held past a loss, which Length does not.
	span := 0
	if r.received {
		span = max(int(seqDiff(r.newestSeq, r.buffer.PlayoutHead()))+1, 0)
	}
	filled := min(span*barWidth/max(int(target), 1), barWidth)
	stats := r.buffer.Stats()
	line := fmt.Sprintf(
		"%-9s [%s%s] %3d/%-3d packets %5dms  est %5.2f Mbps  in %4d pkt/s  out %4d pkt/s  "+
			"skipped %d (late %d)  underflows %d  out-of-order %d  overflows %d",
		r.buffer.State(), strings.Repeat("#", filled), strings.Repeat(".", barWidth-filled),
		span, target, r.buffered().Milliseconds(), float64(r.estimator.GetTargetBitrate())/1e6,
		r.inCount, r.outCount,
		stats.SkippedCount(), r.late, stats.UnderflowCount(), stats.OutOfOrderCount(), stats.OverflowCount(),
	)
	r.inCount, r.outCount = 0, 0

	return line
}

// tsDiff returns a-b for RTP timestamps that may have wrapped around.
func tsDiff(a, b uint32) int32 { return int32(a - b) } //nolint:gosec // wraparound is intended

// seqDiff returns a-b for sequence numbers that may have wrapped around.
func seqDiff(a, b uint16) int16 { return int16(a - b) } //nolint:gosec // wraparound is intended
