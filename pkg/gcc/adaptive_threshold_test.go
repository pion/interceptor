// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAdaptiveThreshold(t *testing.T) {
	type input struct {
		estimate, delta time.Duration
	}
	cases := []struct {
		name     string
		in       []input
		expected []usage
		options  []adaptiveThresholdOption
	}{
		{
			name:     "empty",
			in:       []input{},
			expected: []usage{},
			options:  []adaptiveThresholdOption{},
		},
		{
			name: "firstInputIsAlwaysNormal",
			in: []input{{
				estimate: 1 * time.Second,
				delta:    0,
			}},
			expected: []usage{usageNormal},
			options:  []adaptiveThresholdOption{},
		},
		{
			name: "singleOver",
			in: []input{
				{
					estimate: 0,
					delta:    0,
				},
				{
					estimate: 20 * time.Millisecond,
					delta:    0,
				},
			},
			expected: []usage{usageNormal, usageOver},
			options: []adaptiveThresholdOption{
				setInitialThreshold(10 * time.Millisecond),
			},
		},
		{
			name: "singleNormal",
			in: []input{
				{
					estimate: 0,
					delta:    0,
				},
				{
					estimate: 5 * time.Millisecond,
					delta:    0,
				},
			},
			expected: []usage{usageNormal, usageNormal},
			options: []adaptiveThresholdOption{
				setInitialThreshold(10 * time.Millisecond),
			},
		},
		{
			name: "singleUnder",
			in: []input{
				{
					estimate: 0,
					delta:    0,
				},
				{
					estimate: -20 * time.Millisecond,
					delta:    0,
				},
			},
			expected: []usage{usageNormal, usageUnder},
			options: []adaptiveThresholdOption{
				setInitialThreshold(10 * time.Millisecond),
			},
		},
		{
			name: "increaseThresholdOnOveruse",
			in: []input{
				{
					estimate: 0,
					delta:    0,
				},
				{
					estimate: 25 * time.Millisecond,
					delta:    30 * time.Millisecond,
				},
				{
					estimate: 13 * time.Millisecond,
					delta:    30 * time.Millisecond,
				},
			},
			expected: []usage{usageNormal, usageOver, usageNormal},
			options: []adaptiveThresholdOption{
				setInitialThreshold(20 * time.Millisecond),
			},
		},
		{
			name: "overuseAfterOveruse",
			in: []input{
				{
					estimate: 0,
					delta:    0,
				},
				{
					estimate: 20 * time.Millisecond,
					delta:    30 * time.Millisecond,
				},
				{
					estimate: 30 * time.Millisecond,
					delta:    30 * time.Millisecond,
				},
			},
			expected: []usage{usageNormal, usageOver, usageOver},
			options: []adaptiveThresholdOption{
				setInitialThreshold(10 * time.Millisecond),
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			threshold := newAdaptiveThreshold(tc.options...)
			usages := []usage{}
			for _, in := range tc.in {
				use, _, _ := threshold.compare(in.estimate, in.delta)
				usages = append(usages, use)
			}
			assert.Equal(t, tc.expected, usages, "%v != %v", tc.expected, usages)
		})
	}
}
