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
				CSRC:             []uint32{},
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
			CSRC:             []uint32{},
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
			CSRC:             []uint32{},
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

func TestAttributesGetRTCPHeader(t *testing.T) {
	t.Run("NilHeader", func(t *testing.T) {
		attributes := Attributes{}
		_, err := attributes.GetRTCPHeader(nil)
		assert.Error(t, err)
	})

	t.Run("Present", func(t *testing.T) {
		attributes := Attributes{
			rtcpHeaderKey: &rtcp.Header{
				Padding: false,
				Count:   0,
				Type:    0,
				Length:  0,
			},
		}
		header, err := attributes.GetRTCPHeader(nil)
		assert.NoError(t, err)
		assert.Equal(t, attributes[rtcpHeaderKey], header)
	})

	t.Run("NotPresent", func(t *testing.T) {
		attributes := Attributes{}
		hdr := &rtcp.Header{
			Padding: false,
			Count:   0,
			Type:    0,
			Length:  0,
		}
		buf, err := hdr.Marshal()
		assert.NoError(t, err)
		header, err := attributes.GetRTCPHeader(buf)
		assert.NoError(t, err)
		assert.Equal(t, hdr, header)
	})

	t.Run("NotPresentFromFullRTCPPacket", func(t *testing.T) {
		attributes := Attributes{}
		pkt := rtcp.TransportLayerCC{
			Header: rtcp.Header{
				Padding: false,
				Count:   0,
				Type:    0,
				Length:  0,
			},
			SenderSSRC:         0,
			MediaSSRC:          0,
			BaseSequenceNumber: 0,
			PacketStatusCount:  0,
			ReferenceTime:      0,
			FbPktCount:         0,
			PacketChunks:       []rtcp.PacketStatusChunk{},
			RecvDeltas:         []*rtcp.RecvDelta{},
		}
		buf, err := pkt.Marshal()
		assert.NoError(t, err)
		header, err := attributes.GetRTCPHeader(buf)
		assert.NoError(t, err)
		assert.Equal(t, &pkt.Header, header)
	})
}
