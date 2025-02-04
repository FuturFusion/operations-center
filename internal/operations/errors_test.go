package operations_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/operations"
)

func TestValidationErr_Error(t *testing.T) {
	err := operations.NewValidationErrf("boom!")

	require.Equal(t, "boom!", err.Error())
}
