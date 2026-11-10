// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package nack

import (
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponderInterceptor_RetransmissionHeaderOwnership(t *testing.T) {
	for _, scenario := range []struct {
		name string
		rtx  uint32
	}{{"RTX", 2}, {"same SSRC", 0}} {
		t.Run(scenario.name, func(t *testing.T) {
			factory, err := NewResponderInterceptor()
			require.NoError(t, err)
			instance, err := factory.NewInterceptor("")
			require.NoError(t, err)
			responder, ok := instance.(*ResponderInterceptor)
			require.True(t, ok)
			t.Cleanup(func() { require.NoError(t, responder.Close()) })
			now := time.Unix(1, 0)
			responder.now = func() time.Time { return now }
			writes := 0
			writer := responder.BindLocalStream(&interceptor.StreamInfo{
				SSRC: 1, SSRCRetransmission: scenario.rtx, PayloadTypeRetransmission: 97,
				RTCPFeedback: []interceptor.RTCPFeedback{{Type: "nack"}},
			}, interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, _ interceptor.Attributes) (int, error) {
				writes++
				if writes > 1 {
					assert.Empty(t, header.GetExtension(7))
					assert.Equal(t, []byte{1, 2}, header.GetExtension(3))
					assert.Equal(t, []uint32{11}, header.CSRC)
					require.NoError(t, header.SetExtension(7, []byte{0, 1}))
					header.GetExtension(3)[0] = 9
					header.CSRC[0] = 22
				}

				return header.MarshalSize() + len(payload), nil
			}))
			header := &rtp.Header{SSRC: 1, SequenceNumber: 42, CSRC: []uint32{11}}
			require.NoError(t, header.SetExtension(3, []byte{1, 2}))
			_, err = writer.Write(header, []byte{0xAA}, nil)
			require.NoError(t, err)
			for range 2 {
				now = now.Add(time.Second)
				responder.resendPackets(&rtcp.TransportLayerNack{
					MediaSSRC: 1, Nacks: []rtcp.NackPair{{PacketID: 42}},
				}, nil)
			}
			require.Equal(t, 3, writes)
		})
	}
}
