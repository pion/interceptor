package scream

import "time"

// ReceiverOption can be used to configure SenderInterceptor.
type ReceiverOption func(r *ReceiverInterceptor) error

// ReceiverInterval sets the feedback send interval for the interceptor
func ReceiverInterval(interval time.Duration) ReceiverOption {
	return func(s *ReceiverInterceptor) error {
		s.interval = interval
		return nil
	}
}
