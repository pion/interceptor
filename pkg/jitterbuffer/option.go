// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
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

// WithLoggerFactory sets a logger factory for the interceptor.
func WithLoggerFactory(loggerFactory logging.LoggerFactory) ReceiverInterceptorOption {
	return func(d *ReceiverInterceptor) error {
		d.loggerFactory = loggerFactory

		return nil
	}
}
