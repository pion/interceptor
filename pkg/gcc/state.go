// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import "fmt"

type state int

const (
	stateIncrease state = iota
	stateDecrease
	stateHold
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
