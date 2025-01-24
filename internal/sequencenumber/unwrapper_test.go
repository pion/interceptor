// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package sequencenumber

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		a, b     uint16
		expected bool
	}{
		{
			a:        1,
			b:        0,
			expected: true,
		},
		{
			a:        65534,
			b:        65535,
			expected: false,
		},
		{
			a:        65535,
			b:        65535,
			expected: false,
		},
		{
			a:        0,
			b:        65535,
			expected: true,
		},
		{
			a:        0,
			b:        32767,
			expected: false,
		},
		{
			a:        32770,
			b:        2,
			expected: true,
		},
		{
			a:        3,
			b:        32770,
			expected: false,
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			assert.Equalf(t, tc.expected, isNewer(tc.a, tc.b), "expected isNewer(%v, %v) to be %v", tc.a, tc.b, tc.expected)
		})
	}
}

func TestUnwrapper(t *testing.T) {
	cases := []struct {
		input    []uint16
		expected []int64
	}{
		{
			input:    []uint16{},
			expected: []int64{},
		},
		{
			input:    []uint16{0, 1, 2, 3, 4},
			expected: []int64{0, 1, 2, 3, 4},
		},
		{
			input:    []uint16{65534, 65535, 0, 1, 2},
			expected: []int64{65534, 65535, 65536, 65537, 65538},
		},
		{
			input:    []uint16{32769, 0},
			expected: []int64{32769, 65536},
		},
		{
			input:    []uint16{32767, 0},
			expected: []int64{32767, 0},
		},
		{
			input:    []uint16{0, 1, 4, 3, 2, 5},
			expected: []int64{0, 1, 4, 3, 2, 5},
		},
		{
			input:    []uint16{65534, 0, 1, 65535, 4, 3, 2, 5},
			expected: []int64{65534, 65536, 65537, 65535, 65540, 65539, 65538, 65541},
		},
		{
			input: []uint16{
				0, 32767, 32768, 32769, 32770,
				1, 2, 32765, 32770, 65535,
			},
			expected: []int64{
				0, 32767, 32768, 32769, 32770,
				65537, 65538, 98301, 98306, 131071,
			},
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			u := &Unwrapper{}
			result := []int64{}
			for _, i := range tc.input {
				result = append(result, u.Unwrap(i))
			}
			assert.Equal(t, tc.expected, result)
		})
	}
}
