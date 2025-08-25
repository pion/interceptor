package bwe

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestState(t *testing.T) {
	t.Run("hold", func(t *testing.T) {
		assert.Equal(t, stateDecrease, stateHold.transition(usageOver))
		assert.Equal(t, stateIncrease, stateHold.transition(usageNormal))
		assert.Equal(t, stateHold, stateHold.transition(usageUnder))
	})

	t.Run("increase", func(t *testing.T) {
		assert.Equal(t, stateDecrease, stateIncrease.transition(usageOver))
		assert.Equal(t, stateIncrease, stateIncrease.transition(usageNormal))
		assert.Equal(t, stateHold, stateIncrease.transition(usageUnder))
	})

	t.Run("decrease", func(t *testing.T) {
		assert.Equal(t, stateDecrease, stateDecrease.transition(usageOver))
		assert.Equal(t, stateHold, stateDecrease.transition(usageNormal))
		assert.Equal(t, stateHold, stateDecrease.transition(usageUnder))
	})
}
