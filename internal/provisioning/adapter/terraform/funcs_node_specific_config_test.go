package terraform

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_splitConfig(t *testing.T) {
	nodeSpecificConfigKeys := map[string]map[string]bool{
		"server": {
			"core.https_address": true,
		},
	}

	tests := []struct {
		name string
		in   any
		kind string

		want splitConfigs
	}{
		{
			name: "server",
			in: map[string]string{
				"core.https_address": "foo",
				"global_key":         "bar",
			},
			kind: "server",

			want: splitConfigs{
				Specific: map[string]string{
					"core.https_address": "foo",
				},
				Global: map[string]string{
					"global_key": "bar",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitConfig(nodeSpecificConfigKeys)(tc.in, tc.kind)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_splitConfig_panics(t *testing.T) {
	tests := []struct {
		name string
		in   any
		kind string
	}{
		{
			name: "not a map",
			in:   "not a map",
			kind: "node",
		},
		{
			name: "map without string key",
			in:   map[int]bool{},
			kind: "node",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Panics(t, func() {
				_ = splitConfig(map[string]map[string]bool{})(tc.in, tc.kind)
			})
		})
	}
}
