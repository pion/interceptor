package gcc

import "fmt"

type state int

const (
	increase state = iota
	decrease
	hold
)

func (s state) transition(u usage) state {
	switch s {
	case hold:
		switch u {
		case over:
			return decrease
		case normal:
			return increase
		case under:
			return hold
		}

	case increase:
		switch u {
		case over:
			return decrease
		case normal:
			return increase
		case under:
			return hold
		}

	case decrease:
		switch u {
		case over:
			return decrease
		case normal:
			return hold
		case under:
			return hold
		}
	}
	panic(fmt.Sprintf("state/usage combination should never be reached: %v : %v", s, u))
}

func (s state) String() string {
	switch s {
	case increase:
		return "increase"
	case decrease:
		return "decrease"
	case hold:
		return "hold"
	default:
		return fmt.Sprintf("invalid state: %d", s)
	}
}
