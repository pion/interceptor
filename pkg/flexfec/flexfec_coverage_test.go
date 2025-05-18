// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package flexfec

import (
	"testing"

	"github.com/pion/interceptor/pkg/flexfec/util"
	"github.com/stretchr/testify/assert"
)

func TestMaskExtractors(t *testing.T) {
	tests := []struct {
		name     string
		setBits  []uint32
		mask1    uint16
		mask2    uint32
		mask3    uint64
		mask3_03 uint64
	}{
		{
			name:     "Empty mask",
			setBits:  []uint32{},
			mask1:    0,
			mask2:    0,
			mask3:    0,
			mask3_03: 0,
		},
		{
			name:     "Single bit in each mask",
			setBits:  []uint32{5, 20, 50},
			mask1:    0x200,             // bit 5
			mask2:    0x2000000,         // bit 20
			mask3:    0x800000000000000, // bit 50
			mask3_03: 0x400000000000000, // bit 50
		},
		{
			name:     "Multiple bits in each mask",
			setBits:  []uint32{0, 7, 14, 15, 30, 45, 46, 80, 108, 109},
			mask1:    0x4081,             // bits 0, 7, 14
			mask2:    0x40008001,         // bits 15, 30, 45
			mask3:    0x8000000020000003, // bits 46, 80, 108, 109
			mask3_03: 0x4000000010000001, // bits 46, 80, 108
		},
		{
			name:     "Boundary values",
			setBits:  []uint32{0, 14, 15, 45, 46, 108, 109},
			mask1:    0x4001,             // bits 0, 14
			mask2:    0x40000001,         // bits 15, 45
			mask3:    0x8000000000000003, // bits 46, 108, 109
			mask3_03: 0x4000000000000001, // bits 46, 108
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mask := util.BitArray{}

			for _, bit := range tt.setBits {
				mask.SetBit(bit)
			}

			actualMask1 := extractMask1(mask)
			actualMask2 := extractMask2(mask)
			actualMask3 := extractMask3(mask)
			actualMask3_03 := extractMask3_03(mask)

			assert.Equal(t, tt.mask1, actualMask1, "Mask1 mismatch")
			assert.Equal(t, tt.mask2, actualMask2, "Mask2 mismatch")
			assert.Equal(t, tt.mask3, actualMask3, "Mask3 mismatch")
			assert.Equal(t, tt.mask3_03, actualMask3_03, "Mask3_03 mismatch")
		})
	}
}
