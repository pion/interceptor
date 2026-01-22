// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package interceptor

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMultiError(t *testing.T) {
	rawErrs := []error{
		errors.New("err1"), //nolint
		errors.New("err2"), //nolint
		errors.New("err3"), //nolint
		errors.New("err4"), //nolint
	}
	errs := flattenErrs([]error{
		rawErrs[0],
		nil,
		rawErrs[1],
		flattenErrs([]error{
			rawErrs[2],
		}),
	})
	str := "err1\nerr2\nerr3"

	assert.Equal(t, str, errs.Error(), "String representation doesn't match")

	errIs, ok := errs.(multiError) //nolint
	assert.True(t, ok, "FlattenErrs returns non-multiError")

	for i := 0; i < 3; i++ {
		assert.True(t, errIs.Is(rawErrs[i]), "Should contain error %d", i)
	}
	assert.False(t, errIs.Is(rawErrs[3]), "Should not contain error %d", 3)
}
