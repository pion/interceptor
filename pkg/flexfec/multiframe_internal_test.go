// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package flexfec

import (
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiFrame_RoundTripRecoversLossAcrossFrames(t *testing.T) {
	// 2-packet frames at PL = 0.25: one frame needs 1 FEC packet (33% overhead),
	// two frames need 1 FEC packet for 4 media (20%), so the group spans two frames.
	factory, err := NewFecInterceptor(
		MultiFrame(3),
		ProtectionLevel(0.25),
		MaxExcessOverhead(0),
		DisableRTTAdaptation(),
	)
	require.NoError(t, err)
	icpt, err := factory.NewInterceptor("")
	require.NoError(t, err)

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:                              protectedStreamSSRC,
		ClockRate:                         90000,
		PayloadTypeForwardErrorCorrection: payloadType,
		SSRCForwardErrorCorrection:        ssrc,
	}, icpt)
	defer func() { assert.NoError(t, stream.Close()) }()

	seq := uint16(65533) // also exercise sequence number wrap-around
	for frame := range 3 {
		for i := range 2 {
			pkt := rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    96,
					SequenceNumber: seq,
					Timestamp:      uint32(1000 + 3000*frame), //nolint:gosec
					SSRC:           protectedStreamSSRC,
					Marker:         i == 1,
				},
				Payload: []byte{byte(frame), byte(i), 0xAA, 0xBB, 0xCC},
			}
			require.NoError(t, stream.WriteRTP(&pkt))
			seq++
		}
	}

	var media, fec []rtp.Packet
	for {
		select {
		case p := <-stream.WrittenRTP():
			if p.SSRC == ssrc {
				fec = append(fec, *p)
			} else {
				media = append(media, *p)
			}

			continue
		default:
		}

		break
	}
	require.Len(t, media, 6)
	require.Len(t, fec, 1, "only the first two frames form a closed group")

	group := media[:4]
	assert.NotEqual(t, group[0].Timestamp, group[3].Timestamp, "group must span frames")
	checkAnyPacketCanBeRecovered(t, group, fec)
}

func TestMultiFrame_GapAtFrameBoundaryKeepsFrameRate(t *testing.T) {
	factory, err := NewFecInterceptor(MultiFrame(6), ProtectionLevel(0.1), MaxExcessOverhead(0), DisableRTTAdaptation())
	require.NoError(t, err)
	icpt, err := factory.NewInterceptor("")
	require.NoError(t, err)
	fecInterceptor, ok := icpt.(*FecInterceptor)
	require.True(t, ok)

	writer := icpt.BindLocalStream(&interceptor.StreamInfo{
		SSRC:                              protectedStreamSSRC,
		ClockRate:                         90000,
		PayloadTypeForwardErrorCorrection: payloadType,
		SSRCForwardErrorCorrection:        ssrc,
	}, interceptor.RTPWriterFunc(func(*rtp.Header, []byte, interceptor.Attributes) (int, error) { return 0, nil }))

	// One packet per frame, no marker bits, 3000 ticks apart, and a sequence gap
	// exactly where the third frame starts.
	for i, seq := range []uint16{1, 2, 5, 6, 7} {
		_, err := writer.Write(&rtp.Header{
			SSRC: protectedStreamSSRC, SequenceNumber: seq, Timestamp: uint32(3000 * i), //nolint:gosec
		}, []byte{0x01}, nil)
		require.NoError(t, err)
	}

	stream := fecInterceptor.streams[protectedStreamSSRC]
	stream.mu.Lock()
	defer stream.mu.Unlock()
	assert.InDelta(t, 3000, stream.multiFrame.frameInterval, 0.5, "every frame end was seen")
}

func TestMultiFrame_ZeroRTTIsAMeasurement(t *testing.T) {
	factory, err := NewFecInterceptor(MultiFrame(6), ProtectionLevel(0.1), MaxExcessOverhead(0))
	require.NoError(t, err)
	icpt, err := factory.NewInterceptor("")
	require.NoError(t, err)
	fecInterceptor, ok := icpt.(*FecInterceptor)
	require.True(t, ok)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fecInterceptor.now = func() time.Time { return now }

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:                              protectedStreamSSRC,
		ClockRate:                         90000,
		PayloadTypeForwardErrorCorrection: payloadType,
		SSRCForwardErrorCorrection:        ssrc,
	}, icpt)
	defer func() { assert.NoError(t, stream.Close()) }()

	// The report echoes our sender report with no delay: an RTT of exactly 0.
	stream.ReceiveRTCP([]rtcp.Packet{&rtcp.ReceiverReport{
		Reports: []rtcp.ReceptionReport{{SSRC: protectedStreamSSRC, LastSenderReport: ntp.ToNTP32(now)}},
	}})
	<-stream.ReadRTCP()

	state := &fecInterceptor.streams[protectedStreamSSRC].multiFrame
	state.frameInterval = 3000 // 30 fps

	assert.Equal(t, uint32(1), fecInterceptor.frameLimit(state), "λ = max(1, round(30·0))")
}
