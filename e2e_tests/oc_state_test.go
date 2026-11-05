package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_diffOCState(t *testing.T) {
	t.Parallel()

	reference := ocState{
		configs: map[string]string{
			"security": `{"oidc":{"issuer":""}}`,
			"settings": `{"log_level":"INFO"}`,
		},
		objects: map[string][]string{
			"token":   {"token-1"},
			"channel": {"stable"},
		},
	}

	tests := []struct {
		name    string
		current ocState

		want       ocStateDiff
		wantEmpty  bool
		wantString string
	}{
		{
			name:    "unchanged",
			current: reference,

			want: ocStateDiff{
				added:   map[string][]string{},
				removed: map[string][]string{},
			},
			wantEmpty:  true,
			wantString: "",
		},
		{
			name: "changed config, added and removed objects",
			current: ocState{
				configs: map[string]string{
					"security": `{"oidc":{"issuer":"https://oidc.example.com"}}`,
					"settings": `{"log_level":"INFO"}`,
				},
				objects: map[string][]string{
					"token":   {"token-1", "token-2"},
					"channel": {},
				},
			},

			want: ocStateDiff{
				configs: []string{"security"},
				added: map[string][]string{
					"token": {"token-2"},
				},
				removed: map[string][]string{
					"channel": {"stable"},
				},
			},
			wantEmpty: false,
			wantString: `changed system config "security"
added token [token-2]
removed channel [stable]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := diffOCState(reference, tc.current)

			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantEmpty, got.isEmpty())
			require.Equal(t, tc.wantString, got.String())
		})
	}
}
