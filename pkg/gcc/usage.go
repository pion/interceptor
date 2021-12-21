package gcc

import "fmt"

type usage int

const (
	over usage = iota
	under
	normal
)

func (u usage) String() string {
	switch u {
	case over:
		return "overuse"
	case under:
		return "underuse"
	case normal:
		return "normal"
	default:
		return fmt.Sprintf("invalid usage: %d", u)
	}
}
