// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import "fmt"

type usage int

const (
	usageOver usage = iota
	usageUnder
	usageNormal
)

func (u usage) String() string {
	switch u {
	case usageOver:
		return "overuse"
	case usageUnder:
		return "underuse"
	case usageNormal:
		return "normal"
	default:
		return fmt.Sprintf("invalid usage: %d", u)
	}
}
