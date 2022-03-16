package test

import (
	"time"
)

// MockTicker is a helper to replace time.Ticker for testing purposes.
type MockTicker struct {
	C chan time.Time
}

// Stop stops the MockTicker.
func (t *MockTicker) Stop() {
}

// Ch returns the tickers channel
func (t *MockTicker) Ch() <-chan time.Time {
	return t.C
}

// Tick sends now to the channel
func (t *MockTicker) Tick(now time.Time) {
	t.C <- now
}
