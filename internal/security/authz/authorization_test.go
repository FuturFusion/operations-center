package authz_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestIsDenied(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error

		want bool
	}{
		{
			name: "nil",
			err:  nil,

			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("rejected"),

			want: false,
		},
		{
			name: "final denial",
			err:  authz.Deny(api.StatusErrorf(http.StatusForbidden, "denied")),

			want: true,
		},
		{
			name: "wrapped final denial",
			err:  fmt.Errorf("context: %w", authz.Deny(errors.New("denied"))),

			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			got := authz.IsDenied(tc.err)

			// Assert
			require.Equal(t, tc.want, got, "expect the final denial to be detected")
		})
	}
}

func TestDeny(t *testing.T) {
	t.Parallel()

	// Run test
	err := authz.Deny(api.StatusErrorf(http.StatusForbidden, "denied"))

	// Assert
	require.EqualError(t, err, "denied", "expect the message of the wrapped error")
	require.True(t, api.StatusErrorCheck(err, http.StatusForbidden), "expect the status of the wrapped error")
}
