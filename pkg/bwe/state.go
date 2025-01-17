package bwe

import "fmt"

type state int

const (
	stateDecrease state = -1
	stateHold     state = 0
	stateIncrease state = 1
)

func (s state) transition(u usage) state {
	switch s {
	case stateHold:
		switch u {
		case usageOver:
			return stateDecrease
		case usageNormal:
			return stateIncrease
		case usageUnder:
			return stateHold
		}

	case stateIncrease:
		switch u {
		case usageOver:
			return stateDecrease
		case usageNormal:
			return stateIncrease
		case usageUnder:
			return stateHold
		}

	case stateDecrease:
		switch u {
		case usageOver:
			return stateDecrease
		case usageNormal:
			return stateHold
		case usageUnder:
			return stateHold
		}
	}
	return stateIncrease
}

func (s state) String() string {
	switch s {
	case stateIncrease:
		return "increase"
	case stateDecrease:
		return "decrease"
	case stateHold:
		return "hold"
	default:
		return fmt.Sprintf("invalid state: %d", s)
	}
}
