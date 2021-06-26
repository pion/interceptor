package types

const (
	// BitPerSecond is a data rate of 1 bit per second
	BitPerSecond = DataRate(1)
	// KiloBitPerSecond is a data rate of 1 kilobit per second
	KiloBitPerSecond = 1000 * BitPerSecond
	// MegaBitPerSecond is a data rate of 1 megabit per second
	MegaBitPerSecond = 1000 * KiloBitPerSecond
)

// DataRate in bit per second
type DataRate int

// BitsPerMillisecond returns the datarate in b/ms (bits per millisecond).
func (r DataRate) BitsPerMillisecond() int {
	return int(r / 1000.0)
}

// MaxDataRate returns the maximum of the given DataRates.
func MaxDataRate(a, b DataRate) DataRate {
	if a > b {
		return a
	}
	return b
}

func MinDataRate(a, b DataRate) DataRate {
	if a < b {
		return a
	}
	return b
}
