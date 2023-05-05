// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package ntp

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNTPTimeConverstion(t *testing.T) {
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
