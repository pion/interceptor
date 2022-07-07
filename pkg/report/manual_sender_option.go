package report

import (
	"github.com/pion/logging"
)

// ManualSenderOption can be used to configure ManualSenderInterceptor.
type ManualSenderOption func(r *ManualSenderInterceptor) error

// ManualSenderLog sets a logger for the interceptor.
func ManualSenderLog(log logging.LeveledLogger) ManualSenderOption {
	return func(r *ManualSenderInterceptor) error {
		r.log = log
		return nil
	}
}
