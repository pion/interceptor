// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package flexfec

import (
	"testing"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestFECInterceptorRetainsPacketBytes(t *testing.T) {
	for _, mutate := range []string{"payload", "extension", "csrc"} {
		t.Run(mutate, func(t *testing.T) {
			factory, err := NewFecInterceptor(NumMediaPackets(2), NumFECPackets(1))
			require.NoError(t, err)
			processor, err := factory.NewInterceptor("")
			require.NoError(t, err)
			info := &interceptor.StreamInfo{SSRC: protectedStreamSSRC, SSRCForwardErrorCorrection: ssrc, PayloadTypeForwardErrorCorrection: payloadType}
			t.Cleanup(func() {
				processor.UnbindLocalStream(info)
				require.NoError(t, processor.Close())
			})
			var mediaPackets, fecPackets []rtp.Packet
			writer := processor.BindLocalStream(info, interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, _ interceptor.Attributes) (int, error) {
				wire, marshalErr := (&rtp.Packet{Header: *header, Payload: payload}).Marshal()
				require.NoError(t, marshalErr)
				var packet rtp.Packet
				require.NoError(t, packet.Unmarshal(wire))
				if packet.SSRC == protectedStreamSSRC {
					mediaPackets = append(mediaPackets, packet)
				} else {
					fecPackets = append(fecPackets, packet)
				}

				return len(wire), nil
			}))
			payload := []byte{1, 2, 3, 4}
			extension := []byte{0xAA, 0xBB}
			header := rtp.Header{Version: 2, SSRC: protectedStreamSSRC, PayloadType: 96, SequenceNumber: 100, CSRC: []uint32{123}}
			require.NoError(t, header.SetExtension(1, extension))
			_, err = writer.Write(&header, payload, nil)
			require.NoError(t, err)
			switch mutate {
			case "payload":
				payload[0] = 0xFF
			case "extension":
				extension[0] = 0xFF
			case "csrc":
				header.CSRC[0] = 456
			}
			header.SequenceNumber++
			_, err = writer.Write(&header, payload, nil)
			require.NoError(t, err)
			require.Len(t, mediaPackets, 2)
			require.Len(t, fecPackets, 1)
			checkAnyPacketCanBeRecovered(t, mediaPackets, fecPackets)
		})
	}
}
