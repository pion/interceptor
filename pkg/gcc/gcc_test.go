package gcc

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMinInt(t *testing.T) {
	tests := []struct {
		expected int
		a, b     int
	}{
		{
			expected: 0,
			a:        0,
			b:        100,
		},
		{
			expected: 10,
			a:        10,
			b:        10,
		},
		{
			expected: 1,
			a:        10,
			b:        1,
		},
	}
	for i, tt := range tests {
		tt := tt
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			assert.Equal(t, tt.expected, minInt(tt.a, tt.b))
		})
	}
}

func TestMaxInt(t *testing.T) {
	tests := []struct {
		expected int
		a, b     int
	}{
		{
			expected: 100,
			a:        0,
			b:        100,
		},
		{
			expected: 10,
			a:        10,
			b:        10,
		},
		{
			expected: 10,
			a:        10,
			b:        1,
		},
	}
	for i, tt := range tests {
		tt := tt
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			assert.Equal(t, tt.expected, maxInt(tt.a, tt.b))
		})
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		expected int
		x        int
		min      int
		max      int
	}{
		{
			expected: 50,
			x:        50,
			min:      0,
			max:      100,
		},
		{
			expected: 50,
			x:        50,
			min:      50,
			max:      100,
		},
		{
			expected: 100,
			x:        100,
			min:      0,
			max:      100,
		},
		{
			expected: 50,
			x:        3,
			min:      50,
			max:      100,
		},
		{
			expected: 100,
			x:        150,
			min:      0,
			max:      100,
		},
	}
	for i, tt := range tests {
		tt := tt
		t.Run(fmt.Sprintf("int/%v", i), func(t *testing.T) {
			assert.Equal(t, tt.expected, clampInt(tt.x, tt.min, tt.max))
		})
		t.Run(fmt.Sprintf("duration/%v", i), func(t *testing.T) {
			x := time.Duration(tt.x)
			min := time.Duration(tt.min)
			max := time.Duration(tt.max)
			expected := time.Duration(tt.expected)
			assert.Equal(t, expected, clampDuration(x, min, max))
		})
	}
}
