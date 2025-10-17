// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package flexfec_test

import (
	"testing"

	"github.com/pion/interceptor/pkg/flexfec"
	"github.com/pion/rtp"
)

const (
	payloadType         = uint8(49)
	ssrc                = uint32(867589674)
	protectedStreamSSRC = uint32(476325762)
)

// generateMediaPackets creates a slice of RTP packets with fixed-size payloads.
func generateMediaPackets(n int, startSeq uint16) []rtp.Packet {
	mediaPackets := make([]rtp.Packet, 0, n)
	for i := 0; i < n; i++ {
		payload := []byte{
			// Payload with some random data
			1, 2, 3, 4, 5, byte(i),
		}
		packet := rtp.Packet{
			Header: rtp.Header{
				Marker:         true,
				Extension:      false,
				Version:        2,
				PayloadType:    96,
				SequenceNumber: startSeq + uint16(i), //nolint:gosec // G115
				Timestamp:      3653407706,
				SSRC:           protectedStreamSSRC,
			},
			Payload: payload,
		}

		mediaPackets = append(mediaPackets, packet)
	}

	return mediaPackets
}

// generateMediaPacketsWithSizes creates a slice of RTP packets with varying payload sizes.
func generateMediaPacketsWithSizes(n int, startSeq uint16, minSize, maxSize int) []rtp.Packet {
	mediaPackets := make([]rtp.Packet, 0, n)

	for i := 0; i < n; i++ {
		// Calculate a size that varies between minSize and maxSize based on the packet index
		size := minSize + (i % (maxSize - minSize + 1))

		// Create a payload of the calculated size
		payload := make([]byte, size)
		// Fill with some pattern data
		for j := 0; j < size; j++ {
			payload[j] = byte((j + i) % 256)
		}

		packet := rtp.Packet{
			Header: rtp.Header{
				Marker:         true,
				Extension:      false,
				Version:        2,
				PayloadType:    96,
				SequenceNumber: startSeq + uint16(i), //nolint:gosec // G115
				Timestamp:      3653407706,
				SSRC:           protectedStreamSSRC,
			},
			Payload: payload,
		}

		mediaPackets = append(mediaPackets, packet)
	}

	return mediaPackets
}

// BenchmarkFlexEncoder03_EncodeFec benchmarks the FEC encoding with fixed configurations.
func BenchmarkFlexEncoder03_EncodeFec(b *testing.B) {
	benchmarks := []struct {
		name          string
		mediaPackets  int
		fecPackets    uint32
		sequenceStart uint16
	}{
		{"Small_2FEC", 5, 2, 1000},
		{"Medium_3FEC", 10, 3, 1000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			mediaPackets := generateMediaPackets(bm.mediaPackets, bm.sequenceStart)
			encoder := flexfec.NewFlexEncoder03(payloadType, ssrc)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = encoder.EncodeFec(mediaPackets, bm.fecPackets)
			}
		})
	}
}

// BenchmarkFlexEncoder03_EncodeFecVaryingSizes benchmarks the FEC encoding with varying packet sizes.
func BenchmarkFlexEncoder03_EncodeFecVaryingSizes(b *testing.B) {
	benchmarks := []struct {
		name          string
		mediaPackets  int
		fecPackets    uint32
		minSize       int
		maxSize       int
		sequenceStart uint16
	}{
		{"ManySmall_2FEC", 20, 2, 50, 150, 1000},
		{"ManyMedium_3FEC", 30, 3, 200, 800, 1000},
		{"ManyLarge_2FEC", 40, 2, 900, 1400, 1000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			mediaPackets := generateMediaPacketsWithSizes(bm.mediaPackets, bm.sequenceStart, bm.minSize, bm.maxSize)
			encoder := flexfec.NewFlexEncoder03(payloadType, ssrc)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = encoder.EncodeFec(mediaPackets, bm.fecPackets)
			}
		})
	}
}
