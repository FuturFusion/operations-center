package updateserver

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func TestRedactURLError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error

		wantErrString string
	}{
		{
			name:          "not an url error",
			err:           boom.Error,
			wantErrString: boom.Error.Error(),
		},
		{
			name: "url error with token in query",
			err: &url.Error{
				Op:  "Get",
				URL: "https://updates.example.com/1/update.sjson?token=secret",
				Err: boom.Error,
			},
			wantErrString: `Get "https://updates.example.com/1/update.sjson": boom!`,
		},
		{
			name: "url error with invalid url",
			err: &url.Error{
				Op:  "Get",
				URL: "https://updates.example.com/%zz?token=secret",
				Err: boom.Error,
			},
			wantErrString: `Get "": boom!`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			err := redactURLError(tc.err)

			// Assert
			require.EqualError(t, err, tc.wantErrString, "unexpected redacted error")
			require.ErrorIs(t, err, boom.Error, "expect the wrapped error to be kept")
		})
	}
}
