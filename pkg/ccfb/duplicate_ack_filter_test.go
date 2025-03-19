// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package ccfb

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDuplicateAckFilter(t *testing.T) {
	cases := []struct {
		in     []Report
		expect []Report
	}{
		{
			in:     []Report{},
			expect: []Report{},
		},
		{
			in: []Report{
				{
					SSRCToPacketReports: map[uint32][]PacketReport{
						0: {},
					},
				},
			},
			expect: []Report{
				{
					Arrival:   time.Time{},
					Departure: time.Time{},
					SSRCToPacketReports: map[uint32][]PacketReport{
						0: {},
					},
				},
			},
		},
		{
			in: []Report{
				{
					SSRCToPacketReports: map[uint32][]PacketReport{
						0: {
							{
								SeqNr: 1,
							},
							{
								SeqNr: 2,
							},
						},
					},
				},
				{
					SSRCToPacketReports: map[uint32][]PacketReport{
						0: {
							{
								SeqNr: 1,
							},
							{
								SeqNr: 2,
							},
							{
								SeqNr: 3,
							},
						},
					},
				},
			},
			expect: []Report{
				{
					Arrival:   time.Time{},
					Departure: time.Time{},
					SSRCToPacketReports: map[uint32][]PacketReport{
						0: {
							{
								SeqNr: 1,
							},
							{
								SeqNr: 2,
							},
						},
					},
				},
				{
					Arrival:   time.Time{},
					Departure: time.Time{},
					SSRCToPacketReports: map[uint32][]PacketReport{
						0: {
							{
								SeqNr: 3,
							},
						},
					},
				},
			},
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			daf := NewDuplicateAckFilter()
			for i, m := range tc.in {
				daf.Filter(m)
				assert.Equal(t, tc.expect[i], m)
			}
		})
	}
}
