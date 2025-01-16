package response_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/operations"
	"github.com/FuturFusion/operations-center/internal/response"
)

func TestSmartError(t *testing.T) {
	tt := []struct {
		name string
		err  error

		wantCode    int
		wantMessage string
	}{
		{
			name: "operations validation error",
			err:  operations.NewValidationErrf("foobar"),

			wantCode:    http.StatusBadRequest,
			wantMessage: "foobar",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			resp := response.SmartError(tc.err)

			require.Equal(t, tc.wantCode, resp.Code())
			require.Equal(t, tc.wantMessage, resp.String())
		})
	}
}
