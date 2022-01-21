package rfc8888

import (
	"testing"

	"github.com/muxable/rtptools/pkg/x_range"
)

func TestGetSeqRange(t *testing.T) {
	var tests = []struct {
		input []uint16
		expectedFrom  uint16
		expectedTo    uint16
	}{
		{
			input:    []uint16{1, 2, 3, 4, 5},
			expectedFrom: 1,
			expectedTo: 5,
		},
		{
			input:    []uint16{1, 4, 5},
			expectedFrom: 1,
			expectedTo: 5,
		},
		{
			input:    []uint16{0, 1234, 10000},
			expectedFrom: 0,
			expectedTo: 10000,
		},
		{
			input:    []uint16{65500, 100},
			expectedFrom: 65500,
			expectedTo: 100,
		},
		{
			input:    []uint16{1, 2, 3, 4, 5, 65500},
			expectedFrom: 65500,
			expectedTo: 5,
		},
	}

	for _, test := range tests {
		from, to := x_range.GetSeqRange(test.input)
		if from != test.expectedFrom {
			t.Errorf("getSeqRange(%v) returned %v, expected %v", test.input, from, test.expectedFrom)
		}
		if to != test.expectedTo {
			t.Errorf("getSeqRange(%v) returned %v, expected %v", test.input, to, test.expectedTo)
		}
	}
}
