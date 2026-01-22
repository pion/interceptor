// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package pacing

import (
	"time"

	"golang.org/x/time/rate"
)

type rateLimitPacer struct {
	limiter *rate.Limiter
}

func newRateLimitPacer(initialRate, burst int) *rateLimitPacer {
	return &rateLimitPacer{
		limiter: rate.NewLimiter(rate.Limit(initialRate), burst),
	}
}

func (p *rateLimitPacer) SetRate(r, burst int) {
	p.limiter.SetLimit(rate.Limit(r))
	p.limiter.SetBurst(burst)
}

func (p *rateLimitPacer) Budget(t time.Time) float64 {
	return p.limiter.TokensAt(t)
}

func (p *rateLimitPacer) AllowN(t time.Time, n int) bool {
	return p.limiter.AllowN(t, n)
}
