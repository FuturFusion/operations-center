package domain_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func TestValidationErr_Error(t *testing.T) {
	err := domain.NewValidationErrf("boom!")

	require.Equal(t, "boom!", err.Error())
}

func TestRetryableErrf(t *testing.T) {
	err := fmt.Errorf("Outer wrap: %w", domain.NewRetryableErr(boom.Error))

	var retryableErr domain.ErrRetryable
	require.ErrorAs(t, err, &retryableErr)
}
