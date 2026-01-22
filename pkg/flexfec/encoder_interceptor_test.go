// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package flexfec_test

import (
	"testing"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/interceptor/pkg/flexfec"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

type MockFlexEncoder struct {
	Called        bool
	MediaPackets  []rtp.Packet
	NumFECPackets uint32
	FECPackets    []rtp.Packet
}

func NewMockFlexEncoder(fecPackets []rtp.Packet) *MockFlexEncoder {
	return &MockFlexEncoder{
		Called:     false,
		FECPackets: fecPackets,
	}
}

func (m *MockFlexEncoder) EncodeFec(mediaPackets []rtp.Packet, numFecPackets uint32) []rtp.Packet {
	m.Called = true
	m.MediaPackets = mediaPackets
	m.NumFECPackets = numFecPackets

	return m.FECPackets
}

type MockEncoderFactory struct {
	Called      bool
	PayloadType uint8
	SSRC        uint32
	Encoder     flexfec.FlexEncoder
}

func NewMockEncoderFactory(encoder flexfec.FlexEncoder) *MockEncoderFactory {
	return &MockEncoderFactory{
		Called:  false,
		Encoder: encoder,
	}
}

func (m *MockEncoderFactory) NewEncoder(payloadType uint8, ssrc uint32) flexfec.FlexEncoder {
	m.Called = true
	m.PayloadType = payloadType
	m.SSRC = ssrc

	return m.Encoder
}

func TestFecInterceptor_GenerateAndWriteFecPackets(t *testing.T) {
	fecPackets := []rtp.Packet{
		{
			Header: rtp.Header{
				SSRC:           2000,
				PayloadType:    100,
				SequenceNumber: 1000,
			},
			Payload: []byte{0xFE, 0xC0, 0xDE},
		},
	}
	mockEncoder := NewMockFlexEncoder(fecPackets)
	mockFactory := NewMockEncoderFactory(mockEncoder)

	factory, err := flexfec.NewFecInterceptor(
		flexfec.FECEncoderFactory(mockFactory),
		flexfec.NumMediaPackets(2),
		flexfec.NumFECPackets(1),
	)
	assert.NoError(t, err)

	i, err := factory.NewInterceptor("")
	assert.NoError(t, err)

	info := &interceptor.StreamInfo{
		SSRC:                              1000,
		PayloadTypeForwardErrorCorrection: 100,
		SSRCForwardErrorCorrection:        2000,
	}

	stream := test.NewMockStream(info, i)
	defer assert.NoError(t, stream.Close())

	assert.True(t, mockFactory.Called, "NewEncoder should have been called")
	assert.Equal(t, uint8(100), mockFactory.PayloadType, "Should be called with correct payload type")
	assert.Equal(t, uint32(2000), mockFactory.SSRC, "Should be called with correct SSRC")

	for i := uint16(1); i <= 2; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				SSRC:           1000,
				SequenceNumber: i,
				PayloadType:    96,
			},
			Payload: []byte{0x01, 0x02, 0x03, 0x04},
		}
		err = stream.WriteRTP(packet)
		assert.NoError(t, err)
	}

	var mediaPacketsCount, fecPacketsCount int
	for i := 0; i < 3; i++ {
		select {
		case packet := <-stream.WrittenRTP():
			switch packet.PayloadType {
			case 96:
				mediaPacketsCount++
			case 100:
				fecPacketsCount++
				assert.Equal(t, uint32(2000), packet.SSRC)
				assert.Equal(t, []byte{0xFE, 0xC0, 0xDE}, packet.Payload)
			}
		default:
			assert.Fail(t, "Not enough packets were written")
		}
	}

	assert.Equal(t, 2, mediaPacketsCount, "Should have written 2 media packets")
	assert.Equal(t, 1, fecPacketsCount, "Should have written 1 FEC packet")
	assert.True(t, mockEncoder.Called, "EncodeFec should have been called")
	assert.Equal(t, uint32(1), mockEncoder.NumFECPackets, "Should be called with correct number of FEC packets")
}

func TestFecInterceptor_BypassStreamWhenFecPtAndSsrcAreZero(t *testing.T) {
	fecPackets := []rtp.Packet{
		{
			Header: rtp.Header{
				SSRC:           2000,
				PayloadType:    100,
				SequenceNumber: 1000,
			},
			Payload: []byte{0xFE, 0xC0, 0xDE},
		},
	}
	mockEncoder := NewMockFlexEncoder(fecPackets)
	mockFactory := NewMockEncoderFactory(mockEncoder)

	factory, err := flexfec.NewFecInterceptor(
		flexfec.FECEncoderFactory(mockFactory),
		flexfec.NumMediaPackets(1),
		flexfec.NumFECPackets(1),
	)
	assert.NoError(t, err)

	i, err := factory.NewInterceptor("")
	assert.NoError(t, err)

	info := &interceptor.StreamInfo{
		SSRC:                              1,
		PayloadTypeForwardErrorCorrection: 0,
		SSRCForwardErrorCorrection:        0,
	}

	stream := test.NewMockStream(info, i)
	defer assert.NoError(t, stream.Close())

	packet := &rtp.Packet{
		Header: rtp.Header{
			SSRC:        1,
			PayloadType: 96,
		},
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	}

	err = stream.WriteRTP(packet)
	assert.NoError(t, err)

	select {
	case writtenPacket := <-stream.WrittenRTP():
		assert.Equal(t, packet.SSRC, writtenPacket.SSRC)
		assert.Equal(t, packet.SequenceNumber, writtenPacket.SequenceNumber)
		assert.Equal(t, packet.Payload, writtenPacket.Payload)
	default:
		assert.Fail(t, "No packet was written")
	}

	select {
	case <-stream.WrittenRTP():
		assert.Fail(t, "Only one packet must be received")
	default:
	}

	assert.False(t, mockEncoder.Called, "EncodeFec should not have been called")
}

func TestFecInterceptor_EncodeOnlyPacketsWithMediaSsrc(t *testing.T) {
	mockEncoder := NewMockFlexEncoder(nil)
	mockFactory := NewMockEncoderFactory(mockEncoder)

	factory, err := flexfec.NewFecInterceptor(
		flexfec.FECEncoderFactory(mockFactory),
		flexfec.NumMediaPackets(2),
		flexfec.NumFECPackets(1),
	)
	assert.NoError(t, err)

	i, err := factory.NewInterceptor("")
	assert.NoError(t, err)

	info := &interceptor.StreamInfo{
		SSRC:                              1000,
		PayloadTypeForwardErrorCorrection: 100,
		SSRCForwardErrorCorrection:        2000,
	}

	stream := test.NewMockStream(info, i)
	defer assert.NoError(t, stream.Close())

	mediaPacket := &rtp.Packet{
		Header: rtp.Header{
			SSRC:           1000,
			SequenceNumber: 1,
			PayloadType:    96,
		},
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	}

	nonMediaPacket := &rtp.Packet{
		Header: rtp.Header{
			SSRC:           3000, // Different from mediaSSRC
			SequenceNumber: 2,
			PayloadType:    96,
		},
		Payload: []byte{0x05, 0x06, 0x07, 0x08},
	}

	err = stream.WriteRTP(mediaPacket)
	assert.NoError(t, err)

	err = stream.WriteRTP(nonMediaPacket)
	assert.NoError(t, err)

	// The non-media packet should be passed through without being added to the buffer
	select {
	case writtenPacket := <-stream.WrittenRTP():
		assert.Equal(t, mediaPacket.SSRC, writtenPacket.SSRC)
	default:
		assert.Fail(t, "No media packet was written")
	}

	select {
	case writtenPacket := <-stream.WrittenRTP():
		assert.Equal(t, nonMediaPacket.SSRC, writtenPacket.SSRC)
	default:
		assert.Fail(t, "No non-media packet was written")
	}

	assert.False(t, mockEncoder.Called, "EncodeFec should not have been called")
}

type EncoderFactoryFunc func(payloadType uint8, ssrc uint32) flexfec.FlexEncoder

func (f EncoderFactoryFunc) NewEncoder(payloadType uint8, ssrc uint32) flexfec.FlexEncoder {
	return f(payloadType, ssrc)
}

// nolint:cyclop
func TestFecInterceptor_HandleMultipleStreamsCorrectly(t *testing.T) {
	fecPackets1 := []rtp.Packet{
		{
			Header: rtp.Header{
				SSRC:           2000,
				PayloadType:    100,
				SequenceNumber: 1000,
			},
			Payload: []byte{0xFE, 0xC0, 0xDE},
		},
	}
	mockEncoder1 := NewMockFlexEncoder(fecPackets1)

	fecPackets2 := []rtp.Packet{
		{
			Header: rtp.Header{
				SSRC:           3000,
				PayloadType:    101,
				SequenceNumber: 1000,
			},
			Payload: []byte{0xFE, 0xC0, 0xDE},
		},
	}
	mockEncoder2 := NewMockFlexEncoder(fecPackets2)

	customFactory := EncoderFactoryFunc(func(payloadType uint8, ssrc uint32) flexfec.FlexEncoder {
		if payloadType == 100 && ssrc == 2000 {
			return mockEncoder1
		} else if payloadType == 101 && ssrc == 3000 {
			return mockEncoder2
		}

		return nil
	})

	factory, err := flexfec.NewFecInterceptor(
		flexfec.FECEncoderFactory(customFactory),
		flexfec.NumMediaPackets(2),
	)
	assert.NoError(t, err)

	fecInterceptor, err := factory.NewInterceptor("")
	assert.NoError(t, err)

	info1 := &interceptor.StreamInfo{
		SSRC:                              1000,
		PayloadTypeForwardErrorCorrection: 100,
		SSRCForwardErrorCorrection:        2000,
	}

	info2 := &interceptor.StreamInfo{
		SSRC:                              1001,
		PayloadTypeForwardErrorCorrection: 101,
		SSRCForwardErrorCorrection:        3000,
	}

	stream1 := test.NewMockStream(info1, fecInterceptor)
	defer assert.NoError(t, stream1.Close())

	stream2 := test.NewMockStream(info2, fecInterceptor)
	defer assert.NoError(t, stream2.Close())

	for idx := uint16(1); idx <= 2; idx++ {
		packet1 := &rtp.Packet{
			Header: rtp.Header{
				SSRC:           1000,
				SequenceNumber: idx,
				PayloadType:    96,
			},
			Payload: []byte{0x01, 0x02, 0x03, 0x04},
		}
		err = stream1.WriteRTP(packet1)
		assert.NoError(t, err)

		packet2 := &rtp.Packet{
			Header: rtp.Header{
				SSRC:           1001,
				SequenceNumber: idx,
				PayloadType:    97,
			},
			Payload: []byte{0x05, 0x06, 0x07, 0x08},
		}
		err = stream2.WriteRTP(packet2)
		assert.NoError(t, err)
	}

	assert.True(t, mockEncoder1.Called, "First encoder's EncodeFec should have been called")
	assert.True(t, mockEncoder2.Called, "Second encoder's EncodeFec should have been called")

	mediaPacketsCount1 := 0
	fecPacketsCount1 := 0
	for i := 0; i < 3; i++ {
		select {
		case packet := <-stream1.WrittenRTP():
			switch packet.SSRC {
			case 1000:
				mediaPacketsCount1++
			case 2000:
				fecPacketsCount1++
			}
		default:
			assert.Fail(t, "No packet from stream1")
		}
	}
	assert.Equal(t, 2, mediaPacketsCount1, "Expected 2 media packets for stream1")
	assert.Equal(t, 1, fecPacketsCount1, "Expected 1 FEC packet for stream1")

	mediaPacketsCount2 := 0
	fecPacketsCount2 := 0
	for i := 0; i < 3; i++ {
		select {
		case packet := <-stream2.WrittenRTP():
			switch packet.SSRC {
			case 1001:
				mediaPacketsCount2++
			case 3000:
				fecPacketsCount2++
			}
		default:
			assert.Fail(t, "No packet from stream2")
		}
	}
	assert.Equal(t, 2, mediaPacketsCount2, "Expected 2 media packets for stream2")
	assert.Equal(t, 1, fecPacketsCount2, "Expected 1 FEC packet for stream2")
}
