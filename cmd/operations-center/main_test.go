package main

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
)

func TestMain0Version(t *testing.T) {
	t.Skip("not yet implemented")
	// stdoutBuf := bytes.Buffer{}

	// err := main0([]string{"--version"}, &stdoutBuf, nil, mockEnv{})
	// require.NoError(t, err)

	// require.Equal(t, "0.0.1\n", stdoutBuf.String())
}

func TestFormatError(t *testing.T) {
	tests := []struct {
		name string
		err  error

		want string
	}{
		{
			name: "plain error",
			err:  errors.New("Failed to connect"),

			want: "Error: Failed to connect",
		},
		{
			name: "client error reported by the server",
			err:  &client.ServerError{StatusCode: http.StatusBadRequest, Message: `Server "one" is clustered`, RequestID: "request-id"},

			want: `Error: Server "one" is clustered`,
		},
		{
			name: "client error with a hint",
			err:  &client.ServerError{StatusCode: http.StatusBadRequest, Message: `Server "one" is a member of cluster "two" and can not be deleted`, Hint: "Remove the server from the cluster first."},

			want: "Error: Server \"one\" is a member of cluster \"two\" and can not be deleted\nHint: Remove the server from the cluster first.",
		},
		{
			name: "server error reports the request ID",
			err:  fmt.Errorf("Failed to delete server: %w", &client.ServerError{StatusCode: http.StatusInternalServerError, Message: "boom!", RequestID: "request-id"}),

			want: "Error: Failed to delete server: boom! (request ID request-id)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, formatError(tc.err))
		})
	}
}
