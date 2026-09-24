package openfga

import (
	"encoding/json"
	"os"
	"testing"

	openfga "github.com/openfga/go-sdk"
	"github.com/stretchr/testify/require"
)

func Test_isAuthorizationModelEqual(t *testing.T) {
	loadModel := func(t *testing.T, body []byte) openfga.WriteAuthorizationModelRequest {
		t.Helper()

		var model openfga.WriteAuthorizationModelRequest
		err := json.Unmarshal(body, &model)
		require.NoError(t, err)

		return model
	}

	readFile := func(t *testing.T, name string) []byte {
		t.Helper()

		body, err := os.ReadFile(name)
		require.NoError(t, err)

		return body
	}

	// model_v1_openfga.json is model_v1.json as returned by OpenFGA.
	latestV1 := loadModel(t, readFile(t, "testdata/model_v1_openfga.json"))

	tests := []struct {
		name    string
		builtin []byte

		want bool
	}{
		{
			name:    "equal - same model in OpenFGA representation",
			builtin: readFile(t, "testdata/model_v1.json"),

			want: true,
		},
		{
			name:    "not equal - built in model has changed",
			builtin: []byte(authModel),

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := isAuthorizationModelEqual(loadModel(t, tc.builtin), latestV1)
			require.NoError(t, err)

			require.Equal(t, tc.want, got)
		})
	}
}
