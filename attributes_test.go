// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package interceptor

import (
	"testing"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestAttributesGetRTPHeader(t *testing.T) {
	t.Run("NilHeader", func(t *testing.T) {
		attributes := Attributes{}
		_, err := attributes.GetRTPHeader(nil)
		assert.Error(t, err)
	})

	t.Run("Present", func(t *testing.T) {
		attributes := Attributes{
			rtpHeaderKey: &rtp.Header{
				Version:          0,
				Padding:          false,
				Extension:        false,
				Marker:           false,
				PayloadType:      0,
				SequenceNumber:   0,
				Timestamp:        0,
				SSRC:             0,
				ExtensionProfile: 0,
				Extensions:       nil,
			},
		}
		header, err := attributes.GetRTPHeader(nil)
		assert.NoError(t, err)
		assert.Equal(t, attributes[rtpHeaderKey], header)
	})

	t.Run("NotPresent", func(t *testing.T) {
		attributes := Attributes{}
		hdr := &rtp.Header{
			Version:          0,
			Padding:          false,
			Extension:        false,
			Marker:           false,
			PayloadType:      0,
			SequenceNumber:   0,
			Timestamp:        0,
			SSRC:             0,
			ExtensionProfile: 0,
			Extensions:       nil,
		}
		buf, err := hdr.Marshal()
		assert.NoError(t, err)
		header, err := attributes.GetRTPHeader(buf)
		assert.NoError(t, err)
		assert.Equal(t, hdr, header)
	})

	t.Run("NotPresentFromFullRTPPacket", func(t *testing.T) {
		attributes := Attributes{}
		pkt := &rtp.Packet{Header: rtp.Header{
			Version:          0,
			Padding:          false,
			Extension:        false,
			Marker:           false,
			PayloadType:      0,
			SequenceNumber:   0,
			Timestamp:        0,
			SSRC:             0,
			ExtensionProfile: 0,
			Extensions:       nil,
		}, Payload: make([]byte, 1000)}
		buf, err := pkt.Marshal()
		assert.NoError(t, err)
		header, err := attributes.GetRTPHeader(buf)
		assert.NoError(t, err)
		assert.Equal(t, &pkt.Header, header)
	})
}

func TestAttributesGetRTCPPackets(t *testing.T) {
	t.Run("NilPacket", func(t *testing.T) {
		attributes := Attributes{}
		_, err := attributes.GetRTCPPackets(nil)
		assert.Error(t, err)
	})

	t.Run("Present", func(t *testing.T) {
		attributes := Attributes{
			rtcpPacketsKey: []rtcp.Packet{
				&rtcp.TransportLayerCC{
					Header:             rtcp.Header{Padding: false, Count: 0, Type: 0, Length: 0},
					SenderSSRC:         0,
					MediaSSRC:          0,
					BaseSequenceNumber: 0,
					PacketStatusCount:  0,
					ReferenceTime:      0,
					FbPktCount:         0,
					PacketChunks:       []rtcp.PacketStatusChunk{},
					RecvDeltas:         []*rtcp.RecvDelta{},
				},
			},
		}
		packets, err := attributes.GetRTCPPackets(nil)
		assert.NoError(t, err)
		assert.Equal(t, attributes[rtcpPacketsKey], packets)
	})

	t.Run("NotPresent", func(t *testing.T) {
		attributes := Attributes{}
		sr := &rtcp.SenderReport{
			SSRC:        0,
			NTPTime:     0,
			RTPTime:     0,
			PacketCount: 0,
			OctetCount:  0,
		}
		buf, err := sr.Marshal()
		assert.NoError(t, err)
		packets, err := attributes.GetRTCPPackets(buf)
		assert.NoError(t, err)
		assert.Equal(t, []rtcp.Packet{sr}, packets)
	})
}
