package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
)

func TestValidationErr_Error(t *testing.T) {
	err := domain.NewValidationErrf("boom!")

	require.Equal(t, "boom!", err.Error())
}
