package provisioning_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func TestValidationErr(t *testing.T) {
	err := provisioning.NewValidationErrf("boom")
	require.Equal(t, "boom", err.Error())
}
