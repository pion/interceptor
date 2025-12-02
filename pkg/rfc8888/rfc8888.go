// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package rfc8888 is deprecated. Use ccfb instead.
package rfc8888

import (
	"time"

	"github.com/pion/interceptor/pkg/ccfb"
)

// TickerFactory is a factory to create new tickers.
//
// Deprecated: moved to pkg/ccfb.
type TickerFactory = ccfb.TickerFactory

// SenderInterceptorFactory is a interceptor.Factory for a SenderInterceptor.
//
// Deprecated: moved to pkg/ccfb.
type SenderInterceptorFactory = ccfb.SenderInterceptorFactory

// Deprecated: moved to pkg/ccfb.
func NewSenderInterceptor(opts ...Option) (*SenderInterceptorFactory, error) {
	oo := []ccfb.Option{}
	oo = append(oo, opts...)

	return ccfb.NewSenderInterceptor(oo...)
}

// SenderInterceptor sends congestion control feedback as specified in RFC 8888.
//
// Deprecated: moved to pkg/ccfb.
type SenderInterceptor = ccfb.SenderInterceptor

// An Option is a function that can be used to configure a SenderInterceptor.
//
// Deprecated: moved to pkg/ccfb.
type Option = ccfb.Option

// Deprecated: moved to pkg/ccfb.
func SenderTicker(f TickerFactory) Option {
	return ccfb.SenderTicker(f)
}

// Deprecated: moved to pkg/ccfb.
func SenderNow(f func() time.Time) Option {
	return ccfb.SenderNow(f)
}

// Deprecated: moved to pkg/ccfb.
func SendInterval(interval time.Duration) Option {
	return ccfb.SendInterval(interval)
}

// Recorder records incoming RTP packets and their arrival times. Recorder can
// be used to create feedback reports as defined by RFC 8888.
//
// Deprecated: moved to pkg/ccfb.
type Recorder = ccfb.Recorder

// Deprecated: moved to pkg/ccfb.
func NewRecorder() *Recorder {
	return ccfb.NewRecorder()
}
