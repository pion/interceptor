// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"github.com/pion/logging"
)

// ReceiverInterceptorOption can be used to configure ReceiverInterceptor.
type ReceiverInterceptorOption func(d *ReceiverInterceptor) error

// Log sets a logger for the interceptor.
func Log(log logging.LeveledLogger) ReceiverInterceptorOption {
	return func(d *ReceiverInterceptor) error {
		d.log = log

		return nil
	}
}

// WithSkipMissingPackets returns a ReceiverInterceptorOption that configures the jitter buffer
// to skip missing packets instead of waiting for them.
func WithSkipMissingPackets() ReceiverInterceptorOption {
	return func(d *ReceiverInterceptor) error {
		d.skipMissingPackets = true

		return nil
	}
}
