// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package ntp

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNTPToTimeConverstion(t *testing.T) {
	for i, cc := range []struct {
		ts time.Time
	}{
		{
			ts: time.Now(),
		},
		{
			ts: time.Unix(0, 0),
		},
	} {
		t.Run(fmt.Sprintf("TimeToNTP/%v", i), func(t *testing.T) {
			assert.InDelta(t, cc.ts.UnixNano(), ToTime(ToNTP(cc.ts)).UnixNano(), float64(time.Millisecond.Nanoseconds()))
			assert.InDelta(t, cc.ts.UnixNano(), ToTime32(ToNTP32(cc.ts), cc.ts).UnixNano(), float64(time.Millisecond.Nanoseconds()))
		})
	}
}

func TestTimeToNTPConverstion(t *testing.T) {
	for i, cc := range []struct {
		ts uint64
	}{
		{
			ts: 0,
		},
		{
			ts: 65535,
		},
		{
			ts: 16606669245815957503,
		},
		{
			ts: 9487534653230284800,
		},
	} {
		t.Run(fmt.Sprintf("TimeToNTP/%v", i), func(t *testing.T) {
			assert.Equal(t, cc.ts, ToNTP(ToTime(cc.ts)))
		})
	}
}

func TestNTPTime32(t *testing.T) {
	zero := time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)
	notSoLongAgo := time.Date(2022, time.May, 5, 14, 48, 20, 0, time.UTC)
	for i, cc := range []struct {
		input    time.Time
		expected uint32
	}{
		{
			input:    zero,
			expected: 0,
		},
		{
			input:    zero.Add(time.Second),
			expected: 1 << 16,
		},
		{
			input:    notSoLongAgo,
			expected: uint32(uint(notSoLongAgo.Sub(zero).Seconds())&0xffff) << 16,
		},
		{
			input:    zero.Add(400 * time.Millisecond),
			expected: 26214,
		},
		{
			input:    zero.Add(1400 * time.Millisecond),
			expected: 1<<16 + 26214,
		},
	} {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			res := ToNTP32(cc.input)
			assert.Equalf(t, cc.expected, res, "%b != %b", cc.expected, res)
		})
	}
}
