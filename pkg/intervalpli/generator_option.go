package intervalpli

import (
	"time"

	"github.com/pion/logging"
)

// GeneratorOption can be used to configure GeneratorInterceptor.
type GeneratorOption func(r *GeneratorInterceptor) error

// GeneratorLog sets a logger for the interceptor.
func GeneratorLog(log logging.LeveledLogger) GeneratorOption {
	return func(r *GeneratorInterceptor) error {
		r.log = log
		return nil
	}
}

// GeneratorInterval sets send interval for the interceptor.
func GeneratorInterval(interval time.Duration) GeneratorOption {
	return func(r *GeneratorInterceptor) error {
		r.interval = interval
		return nil
	}
}
