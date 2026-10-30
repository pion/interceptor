// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package flexfec_test

import (
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/interceptor/pkg/flexfec"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	mfMediaSSRC = uint32(1000)
	mfFECSSRC   = uint32(2000)
	mfMediaPT   = uint8(96)
	mfFECPT     = uint8(100)
	mfTicks30   = uint32(3000) // 30 fps at 90 kHz
)

type encodeCall struct {
	seqs   []uint16
	numFEC uint32
}

// recordingEncoder records every EncodeFec call and returns numFEC placeholder FEC packets.
type recordingEncoder struct {
	calls []encodeCall
}

func (e *recordingEncoder) EncodeFec(mediaPackets []rtp.Packet, numFecPackets uint32) []rtp.Packet {
	call := encodeCall{numFEC: numFecPackets}
	for _, p := range mediaPackets {
		call.seqs = append(call.seqs, p.SequenceNumber)
	}
	e.calls = append(e.calls, call)

	out := make([]rtp.Packet, numFecPackets)
	for i := range out {
		out[i] = rtp.Packet{Header: rtp.Header{SSRC: mfFECSSRC, PayloadType: mfFECPT}}
	}

	return out
}

type recordingFactory struct{ enc *recordingEncoder }

func (f recordingFactory) NewEncoder(uint8, uint32) flexfec.FlexEncoder { return f.enc }

// mfSender writes frames into a mock stream with consecutive sequence numbers.
type mfSender struct {
	t      *testing.T
	stream *test.MockStream
	seq    uint16
	ts     uint32
}

func newMFSender(t *testing.T, opts ...flexfec.FecOption) (*mfSender, *recordingEncoder) {
	t.Helper()

	enc := &recordingEncoder{}
	factory, err := flexfec.NewFecInterceptor(append([]flexfec.FecOption{
		flexfec.FECEncoderFactory(recordingFactory{enc}),
	}, opts...)...)
	require.NoError(t, err)

	i, err := factory.NewInterceptor("")
	require.NoError(t, err)

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:                              mfMediaSSRC,
		ClockRate:                         90000,
		PayloadTypeForwardErrorCorrection: mfFECPT,
		SSRCForwardErrorCorrection:        mfFECSSRC,
	}, i)
	t.Cleanup(func() { assert.NoError(t, stream.Close()) })

	return &mfSender{t: t, stream: stream, seq: 1, ts: 90000}, enc
}

// frame writes a frame of n packets; the marker bit is set on the last one when marker is true.
func (s *mfSender) frame(n int, marker bool) {
	s.t.Helper()

	for i := range n {
		require.NoError(s.t, s.stream.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				SSRC:           mfMediaSSRC,
				PayloadType:    mfMediaPT,
				SequenceNumber: s.seq,
				Timestamp:      s.ts,
				Marker:         marker && i == n-1,
			},
			Payload: []byte{0x01, 0x02, 0x03},
		}))
		s.seq++
	}
	s.ts += mfTicks30
}

// written drains the packets written so far and returns their payload types in order.
func (s *mfSender) written() []uint8 {
	var pts []uint8
	for {
		select {
		case p := <-s.stream.WrittenRTP():
			pts = append(pts, p.PayloadType)
		default:
			return pts
		}
	}
}

func seqRange(from, to uint16) []uint16 {
	var out []uint16
	for s := from; s <= to; s++ {
		out = append(out, s)
	}

	return out
}

func TestMultiFrame_GroupsFramesUntilOverheadNearTarget(t *testing.T) {
	// PL = 0.2. A 2-packet frame needs 1 FEC packet (33% overhead, 13% too much),
	// two such frames hit 20% exactly, so groups span two frames.
	sender, enc := newMFSender(t,
		flexfec.MultiFrame(6),
		flexfec.ProtectionLevel(0.2),
		flexfec.MaxExcessOverhead(0.05),
		flexfec.DisableRTTAdaptation(),
	)

	for range 4 {
		sender.frame(2, true)
	}

	require.Len(t, enc.calls, 2)
	assert.Equal(t, encodeCall{seqs: seqRange(1, 4), numFEC: 1}, enc.calls[0])
	assert.Equal(t, encodeCall{seqs: seqRange(5, 8), numFEC: 1}, enc.calls[1])

	// FEC follows the last media packet of each group.
	m, f := mfMediaPT, mfFECPT
	assert.Equal(t, []uint8{m, m, m, m, f, m, m, m, m, f}, sender.written())
}

func TestMultiFrame_ProtectionLevelDerivedFromPacketCounts(t *testing.T) {
	// NumMediaPackets(2)/NumFECPackets(1) means PL = 1/3, met by a single 2-packet frame.
	sender, enc := newMFSender(t,
		flexfec.MultiFrame(6),
		flexfec.NumMediaPackets(2),
		flexfec.NumFECPackets(1),
		flexfec.MaxExcessOverhead(0),
		flexfec.DisableRTTAdaptation(),
	)

	sender.frame(2, true)
	sender.frame(2, true)

	require.Len(t, enc.calls, 2)
	assert.Equal(t, encodeCall{seqs: seqRange(1, 2), numFEC: 1}, enc.calls[0])
}

func TestMultiFrame_FrameLimitCapsGroup(t *testing.T) {
	// PL = 0.1 with 1-packet frames is only met after 9 frames, so λ0 = 3 decides.
	sender, enc := newMFSender(t,
		flexfec.MultiFrame(3),
		flexfec.ProtectionLevel(0.1),
		flexfec.MaxExcessOverhead(0),
		flexfec.DisableRTTAdaptation(),
	)

	for range 6 {
		sender.frame(1, true)
	}

	require.Len(t, enc.calls, 2)
	assert.Equal(t, encodeCall{seqs: seqRange(1, 3), numFEC: 1}, enc.calls[0])
	assert.Equal(t, encodeCall{seqs: seqRange(4, 6), numFEC: 1}, enc.calls[1])
}

func TestMultiFrame_FrameLimitFollowsRTT(t *testing.T) {
	opts := []flexfec.FecOption{
		flexfec.MultiFrame(6),
		flexfec.ProtectionLevel(0.1),
		flexfec.MaxExcessOverhead(0),
	}

	t.Run("no RTT yet uses max frames", func(t *testing.T) {
		sender, enc := newMFSender(t, opts...)
		for range 6 {
			sender.frame(1, true)
		}
		require.Len(t, enc.calls, 1)
		assert.Len(t, enc.calls[0].seqs, 6)
	})

	t.Run("100ms RTT at 30fps gives 3 frames", func(t *testing.T) {
		sender, enc := newMFSender(t, opts...)

		const rttNTP = 1 << 16 / 10 // 100 ms in 1/65536 s
		sender.stream.ReceiveRTCP([]rtcp.Packet{&rtcp.ReceiverReport{
			Reports: []rtcp.ReceptionReport{{
				SSRC:             mfMediaSSRC,
				LastSenderReport: ntp.ToNTP32(time.Now()) - rttNTP,
			}},
		}})
		<-sender.stream.ReadRTCP()

		for range 6 {
			sender.frame(1, true)
		}
		require.Len(t, enc.calls, 2)
		assert.Equal(t, seqRange(1, 3), enc.calls[0].seqs)
		assert.Equal(t, seqRange(4, 6), enc.calls[1].seqs)
	})

	t.Run("short RTT gives single-frame groups", func(t *testing.T) {
		sender, enc := newMFSender(t, opts...)

		sender.stream.ReceiveRTCP([]rtcp.Packet{&rtcp.ReceiverReport{
			Reports: []rtcp.ReceptionReport{{
				SSRC:             mfMediaSSRC,
				LastSenderReport: ntp.ToNTP32(time.Now()) - 1<<16/100, // 10 ms
			}},
		}})
		<-sender.stream.ReadRTCP()

		// The first frame has no frame rate estimate yet, so λ0 applies;
		// from the second frame on λ = max(1, round(30·0.01)) = 1.
		for range 3 {
			sender.frame(1, true)
		}
		require.Len(t, enc.calls, 2)
		assert.Equal(t, seqRange(1, 2), enc.calls[0].seqs)
		assert.Equal(t, seqRange(3, 3), enc.calls[1].seqs)
	})
}

func TestMultiFrame_TimestampChangeEndsFrameWithoutMarker(t *testing.T) {
	sender, enc := newMFSender(t,
		flexfec.MultiFrame(6),
		flexfec.ProtectionLevel(1.0/3),
		flexfec.MaxExcessOverhead(0),
		flexfec.DisableRTTAdaptation(),
	)

	sender.frame(2, false)
	assert.Empty(t, enc.calls, "frame end is not known yet")

	sender.frame(1, false)
	require.Len(t, enc.calls, 1)
	assert.Equal(t, seqRange(1, 2), enc.calls[0].seqs)

	// The FEC for the first frame goes out after the packet that revealed the frame end.
	m, f := mfMediaPT, mfFECPT
	assert.Equal(t, []uint8{m, m, m, f}, sender.written())
}

func TestMultiFrame_SequenceGapFlushesGroup(t *testing.T) {
	sender, enc := newMFSender(t,
		flexfec.MultiFrame(6),
		flexfec.ProtectionLevel(0.1),
		flexfec.MaxExcessOverhead(0),
		flexfec.DisableRTTAdaptation(),
	)

	sender.frame(2, true)
	sender.seq += 3
	sender.frame(1, true)

	require.Len(t, enc.calls, 1)
	assert.Equal(t, seqRange(1, 2), enc.calls[0].seqs)
}

func TestMultiFrame_GroupSizeIsBounded(t *testing.T) {
	sender, enc := newMFSender(t,
		flexfec.MultiFrame(6),
		flexfec.ProtectionLevel(0.01),
		flexfec.MaxExcessOverhead(0),
		flexfec.DisableRTTAdaptation(),
	)

	sender.frame(120, true)

	require.NotEmpty(t, enc.calls)
	assert.Len(t, enc.calls[0].seqs, 109, "FlexFEC-03 masks cover at most 109 packets")
}

func TestMultiFrame_InvalidOptions(t *testing.T) {
	for name, opt := range map[string]flexfec.FecOption{
		"zero frames":     flexfec.MultiFrame(0),
		"zero PL":         flexfec.ProtectionLevel(0),
		"PL of one":       flexfec.ProtectionLevel(1),
		"negative excess": flexfec.MaxExcessOverhead(-0.1),
	} {
		t.Run(name, func(t *testing.T) {
			factory, err := flexfec.NewFecInterceptor(opt)
			require.NoError(t, err)
			_, err = factory.NewInterceptor("")
			assert.Error(t, err)
		})
	}

	t.Run("no FEC packets to derive PL", func(t *testing.T) {
		factory, err := flexfec.NewFecInterceptor(flexfec.MultiFrame(3), flexfec.NumFECPackets(0))
		require.NoError(t, err)
		_, err = factory.NewInterceptor("")
		assert.Error(t, err)
	})
}
