// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package util implements utilities to better support Fec decoding / encoding.
package util

// BitArray provides support for bitmask manipulations.
type BitArray struct {
	bytes []byte
}

// NewBitArray returns a new BitArray. It takes sizeBits as parameter which represents
// the size of the underlying bitmask.
func NewBitArray(sizeBits uint32) BitArray {
	var sizeBytes uint32
	if sizeBits%8 == 0 {
		sizeBytes = sizeBits / 8
	} else {
		sizeBytes = sizeBits/8 + 1
	}

	return BitArray{
		bytes: make([]byte, sizeBytes),
	}
}

// SetBit sets a bit to the specified bit value on the bitmask.
func (b *BitArray) SetBit(bitIndex uint32, bitValue uint32) {
	byteIndex := bitIndex / 8
	bitOffset := uint(bitIndex % 8)

	// Set the specific bit to 1 using bitwise OR
	if bitValue == 1 {
		b.bytes[byteIndex] |= 1 << bitOffset
	} else {
		b.bytes[byteIndex] |= 0 << bitOffset
	}
}

// GetBit returns the bit value at a specified index of the bitmask.
func (b *BitArray) GetBit(bitIndex uint32) uint8 {
	return b.bytes[bitIndex/8]
}

// Marshal returns the underlying bitmask.
func (b *BitArray) Marshal() []byte {
	return b.bytes
}

// GetBitValue returns a subsection of the bitmask.
func (b *BitArray) GetBitValue(i int, j int) uint64 {
	if i < 0 || i >= len(b.bytes)*8 || j < 0 || j >= len(b.bytes)*8 || i > j {
		return 0
	}

	startByte := i / 8
	startBit := i % 8
	endByte := j / 8

	// Create a slice containing the bytes to extract
	subArray := b.bytes[startByte : endByte+1]

	// Initialize the result value
	var result uint64

	// Loop through the bytes and concatenate the bits
	for idx, b := range subArray {
		if idx == 0 {
			b <<= uint(startBit)
		}
		result |= uint64(b)
	}

	// Mask the bits that are not part of the desired range
	result &= (1<<uint(j-i+1) - 1)

	return result
}
