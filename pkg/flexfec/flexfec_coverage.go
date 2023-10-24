// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package flexfec

import (
	"github.com/pion/interceptor/pkg/flexfec/util"
	"github.com/pion/rtp"
)

// Maximum number of media packets that can be protected by a single FEC packet.
// We are not supporting the possibility of having an FEC packet protect multiple
// SSRC source packets for now.
// https://datatracker.ietf.org/doc/html/rfc8627#section-4.2.2.1
const (
	MaxMediaPackets uint32 = 110
	MaxFecPackets   uint32 = MaxMediaPackets
)

// ProtectionCoverage defines the map of RTP packets that individual Fec packets protect.
type ProtectionCoverage struct {
	// Array of masks, each mask capable of covering up to maxMediaPkts = 110.
	// A mask is represented as a grouping of bytes where each individual bit
	// represents the coverage for the media packet at the corresponding index.
	packetMasks     [MaxFecPackets]util.BitArray
	numFecPackets   uint32
	numMediaPackets uint32
	mediaPackets    []rtp.Packet
}

// NewCoverage returns a new ProtectionCoverage object. numFecPackets represents the number of
// Fec packets that we will be generating to cover the list of mediaPackets. This allows us to know
// how big the underlying map should be.
func NewCoverage(mediaPackets []rtp.Packet, numFecPackets uint32) *ProtectionCoverage {
	numMediaPackets := uint32(len(mediaPackets))

	// Basic sanity checks
	if numMediaPackets <= 0 || numMediaPackets > MaxMediaPackets {
		return nil
	}

	// We allocate the biggest array of bitmasks that respects the max constraints.
	var packetMasks [MaxFecPackets]util.BitArray
	for i := 0; i < int(MaxFecPackets); i++ {
		packetMasks[i] = util.NewBitArray(MaxMediaPackets)
	}

	// Generate FEC bit mask where numFecPackets FEC packets are covering numMediaPackets Media packets.
	// In the packetMasks array, each FEC packet is represented by a single BitArray, each bit in a given BitArray
	// corresponds to a specific Media packet.
	// Ex: Row I, Col J is set to 1 -> FEC packet I will protect media packet J.
	for fecPacketIndex := uint32(0); fecPacketIndex < numFecPackets; fecPacketIndex++ {
		// We use an interleaved method to determine coverage. Given N FEC packets, Media packet X will be
		// covered by FEC packet X % N.
		for mediaPacketIndex := uint32(0); mediaPacketIndex < numMediaPackets; mediaPacketIndex++ {
			coveringFecPktIndex := mediaPacketIndex % numFecPackets
			packetMasks[coveringFecPktIndex].SetBit(mediaPacketIndex, 1)
		}
	}

	return &ProtectionCoverage{
		packetMasks:     packetMasks,
		numFecPackets:   numFecPackets,
		numMediaPackets: numMediaPackets,
		mediaPackets:    mediaPackets,
	}
}

// ResetCoverage clears the underlying map so that we can reuse it for new batches of RTP packets.
func (p *ProtectionCoverage) ResetCoverage() {
	for i := uint32(0); i < MaxFecPackets; i++ {
		for j := uint32(0); j < MaxMediaPackets; j++ {
			p.packetMasks[i].SetBit(j, 0)
		}
	}
}

// GetCoveredBy returns an iterator over RTP packets that are protected by the specified Fec packet index.
func (p *ProtectionCoverage) GetCoveredBy(fecPacketIndex uint32) *util.MediaPacketIterator {
	coverage := make([]uint32, 0, p.numMediaPackets)
	for mediaPacketIndex := uint32(0); mediaPacketIndex < p.numMediaPackets; mediaPacketIndex++ {
		if p.packetMasks[fecPacketIndex].GetBit(mediaPacketIndex) == 1 {
			coverage = append(coverage, mediaPacketIndex)
		}
	}
	return util.NewMediaPacketIterator(p.mediaPackets, coverage)
}

// MarshalBitmasks returns the underlying bitmask that defines which media packets are protected by the
// specified fecPacketIndex.
func (p *ProtectionCoverage) MarshalBitmasks(fecPacketIndex uint32) []byte {
	return p.packetMasks[fecPacketIndex].Marshal()
}

// ExtractMask1 returns the first section of the bitmask as defined by the FEC header.
// https://datatracker.ietf.org/doc/html/rfc8627#section-4.2.2.1
func (p *ProtectionCoverage) ExtractMask1(fecPacketIndex uint32) uint16 {
	return uint16(p.packetMasks[fecPacketIndex].GetBitValue(0, 14))
}

// ExtractMask2 returns the second section of the bitmask as defined by the FEC header.
// https://datatracker.ietf.org/doc/html/rfc8627#section-4.2.2.1
func (p *ProtectionCoverage) ExtractMask2(fecPacketIndex uint32) uint32 {
	return uint32(p.packetMasks[fecPacketIndex].GetBitValue(15, 45))
}

// ExtractMask3 returns the third section of the bitmask as defined by the FEC header.
// https://datatracker.ietf.org/doc/html/rfc8627#section-4.2.2.1
func (p *ProtectionCoverage) ExtractMask3(fecPacketIndex uint32) uint64 {
	return p.packetMasks[fecPacketIndex].GetBitValue(46, 109)
}
