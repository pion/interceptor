// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package flexfec

import (
	"testing"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
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
