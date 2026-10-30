// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package flexfec

import (
	"errors"
	"math"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/interceptor/internal/rtpbuffer"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

const (
	// maxGroupPackets is the largest protection group the FlexFEC-03 header can describe:
	// the three masks carry 15 + 31 + 63 = 109 bits.
	maxGroupPackets = 109

	defaultMaxExcessOverhead = 0.2
	defaultClockRate         = 90000

	// Smoothing factors for the frame interval and RTT estimates.
	frameIntervalAlpha = 0.1
	rttAlpha           = 0.125
)

var (
	errInvalidMaxFrames         = errors.New("flexfec: max frames must be at least 1")
	errInvalidProtectionLevel   = errors.New("flexfec: protection level must be in (0, 1)")
	errInvalidMaxExcessOverhead = errors.New("flexfec: max excess overhead must be non-negative")
)

// multiFrameState is the per-stream state used in MultiFrame mode. It is guarded by streamState.mu.
type multiFrameState struct {
	clockRate uint32

	// Number of complete frames in the current protection group.
	frames uint32

	// Highest sequence number buffered so far, kept across groups so that
	// retransmitted or reordered packets can be recognized.
	lastSeq uint16
	haveSeq bool

	// Frame rate estimation from RTP timestamps of frame ends.
	lastFrameTS   uint32
	haveFrameTS   bool
	frameInterval float64 // in RTP clock ticks, 0 while unknown

	// Smoothed RTT from RTCP receiver reports. A measured RTT of 0 is valid,
	// so whether one was measured is tracked separately.
	rtt     time.Duration
	haveRTT bool
}

// fecCount returns the number of FEC packets that brings a group of k media packets closest to
// the target protection level, with at least one FEC packet and at most one per media packet.
func (r *FecInterceptor) fecCount(k int) uint32 {
	pl := r.protectionLevel
	m := int(math.Round(float64(k) * pl / (1 - pl)))

	return uint32(min(max(m, 1), k)) //nolint:gosec // G115, bounded by maxGroupPackets
}

// frameLimit returns λ = max(1, min(f·RTT, maxFrames)).
func (r *FecInterceptor) frameLimit(state *multiFrameState) uint32 {
	if r.disableRTTAdaptation || !state.haveRTT || state.frameInterval <= 0 {
		return r.maxFrames
	}

	fps := float64(state.clockRate) / state.frameInterval
	lambda := math.Round(fps * state.rtt.Seconds())

	return uint32(min(max(lambda, 1), float64(r.maxFrames)))
}

// onFrameEnd records that the last buffered packet completed a frame with the given timestamp.
func (state *multiFrameState) onFrameEnd(timestamp uint32) {
	state.frames++

	if state.haveFrameTS {
		delta := timestamp - state.lastFrameTS // wraps correctly for uint32
		// Ignore duplicate timestamps and gaps of a second or more (pauses, reordering).
		if delta > 0 && delta < state.clockRate {
			if state.frameInterval == 0 {
				state.frameInterval = float64(delta)
			} else {
				state.frameInterval += frameIntervalAlpha * (float64(delta) - state.frameInterval)
			}
		}
	}
	state.lastFrameTS = timestamp
	state.haveFrameTS = true
}

// isStale reports whether a packet should bypass protection: retransmissions that keep the
// media SSRC (no RTX) and late reordered packets are already covered by an earlier group or
// the NACK path, and must not break up the current group.
func (state *multiFrameState) isStale(header *rtp.Header, attributes interceptor.Attributes) bool {
	if _, isResend := attributes[rtpbuffer.ResendKey{}]; isResend {
		return true
	}

	return state.haveSeq && int16(header.SequenceNumber-state.lastSeq) <= 0 //nolint:gosec // G115, wrap-aware diff
}

// flushGroup protects everything buffered for the stream and starts a new group.
// The caller must hold stream.mu.
func (r *FecInterceptor) flushGroup(stream *streamState) []rtp.Packet {
	if len(stream.packetBuffer) == 0 {
		return nil
	}

	fecPackets := stream.flexFecEncoder.EncodeFec(stream.packetBuffer, r.fecCount(len(stream.packetBuffer)))
	stream.packetBuffer = nil
	stream.multiFrame.frames = 0

	return fecPackets
}

// maybeCloseGroup is called after a frame ends and closes the group if it spans enough frames
// or its overhead is close enough to the target.
// The caller must hold stream.mu.
func (r *FecInterceptor) maybeCloseGroup(stream *streamState) []rtp.Packet {
	k := len(stream.packetBuffer)
	if k == 0 {
		return nil
	}

	m := r.fecCount(k)
	excess := float64(m)/float64(k+int(m)) - r.protectionLevel

	if stream.multiFrame.frames >= r.frameLimit(&stream.multiFrame) || excess <= r.maxExcessOverhead {
		return r.flushGroup(stream)
	}

	return nil
}

// collectMultiFrame adds a media packet to the current protection group and returns the FEC
// packets to send after it, if the group was closed.
// The caller must hold stream.mu.
func (r *FecInterceptor) collectMultiFrame(
	stream *streamState, header *rtp.Header, payload []byte, attributes interceptor.Attributes,
) []rtp.Packet {
	state := &stream.multiFrame

	if state.isStale(header, attributes) {
		return nil
	}

	var fecPackets []rtp.Packet

	if n := len(stream.packetBuffer); n > 0 {
		last := &stream.packetBuffer[n-1]
		frameEnded := header.Timestamp != last.Timestamp && !last.Marker
		switch {
		case header.SequenceNumber != last.SequenceNumber+1:
			// Packets were skipped. The encoder needs consecutive sequence numbers, so
			// protect what we have rather than lose protection for the whole group.
			if frameEnded {
				state.onFrameEnd(last.Timestamp) // keep the frame rate estimate accurate
			}
			fecPackets = r.flushGroup(stream)
		case frameEnded:
			// A new frame started although the previous one had no marker bit.
			state.onFrameEnd(last.Timestamp)
			fecPackets = r.maybeCloseGroup(stream)
		}
	}

	state.lastSeq = header.SequenceNumber
	state.haveSeq = true

	// Packets may stay buffered for several frames, longer than callers can be
	// expected to keep their buffers untouched, so keep a private copy.
	stream.packetBuffer = append(stream.packetBuffer, *rtp.Packet{Header: *header, Payload: payload}.Clone())

	if header.Marker {
		stream.multiFrame.onFrameEnd(header.Timestamp)
		fecPackets = append(fecPackets, r.maybeCloseGroup(stream)...)
	}

	if len(stream.packetBuffer) >= maxGroupPackets {
		fecPackets = append(fecPackets, r.flushGroup(stream)...)
	}

	return fecPackets
}

// BindRTCPReader lets you modify any incoming RTCP packets. In MultiFrame mode with RTT
// adaptation, receiver reports are used to estimate the RTT of each protected stream.
func (r *FecInterceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	if !r.multiFrame || r.disableRTTAdaptation {
		return reader
	}

	return interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		i, attr, err := reader.Read(b, a)
		if err != nil {
			return 0, nil, err
		}

		if attr == nil {
			attr = make(interceptor.Attributes)
		}
		pkts, err := attr.GetRTCPPackets(b[:i])
		if err != nil {
			return 0, nil, err
		}

		for _, pkt := range pkts {
			switch report := pkt.(type) {
			case *rtcp.ReceiverReport:
				r.updateRTT(report.Reports)
			case *rtcp.SenderReport:
				r.updateRTT(report.Reports)
			}
		}

		return i, attr, nil
	})
}

func (r *FecInterceptor) updateRTT(reports []rtcp.ReceptionReport) {
	now := ntp.ToNTP32(r.now())

	for _, report := range reports {
		if report.LastSenderReport == 0 {
			continue
		}

		r.mu.Lock()
		stream := r.streams[report.SSRC]
		r.mu.Unlock()
		if stream == nil {
			continue
		}

		rtt := now - report.LastSenderReport - report.Delay
		if rtt >= 1<<31 { // negative, clock or report is off
			continue
		}
		sample := time.Duration(rtt) * time.Second / (1 << 16)

		stream.mu.Lock()
		if !stream.multiFrame.haveRTT {
			stream.multiFrame.rtt = sample
			stream.multiFrame.haveRTT = true
		} else {
			stream.multiFrame.rtt += time.Duration(rttAlpha * float64(sample-stream.multiFrame.rtt))
		}
		stream.mu.Unlock()
	}
}
