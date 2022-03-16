package rfc8888

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			u := &unwrapper{}
			result := []int64{}
			for _, i := range tc.input {
				result = append(result, u.unwrap(i))
			}
			assert.Equal(t, tc.expected, result)
		})
	}
}
