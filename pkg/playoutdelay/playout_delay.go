// Package playoutdelay implements the playout delay header extension.
package playoutdelay

import (
	"errors"
)

var (
	errPlayoutDelayInvalidValue = errors.New("invalid playout delay value")
)
