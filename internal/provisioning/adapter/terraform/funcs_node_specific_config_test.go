package terraform

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_splitConfig(t *testing.T) {
	tests := []struct {
		name string
		in   any
		kind string

		want splitConfigs
	}{
		{
			name: "node",
			in: map[string]string{
				"core.https_address": "foo",
				"global_key":         "bar",
			},
			kind: "node",

			want: splitConfigs{
				Specific: map[string]string{
					"core.https_address": "foo",
				},
				Global: map[string]string{
					"global_key": "bar",
				},
			},
		},
		{
			name: "storage",
			in: map[string]string{
				"size":       "foo",
				"global_key": "bar",
			},
			kind: "storage",

			want: splitConfigs{
				Specific: map[string]string{
					"size": "foo",
				},
				Global: map[string]string{
					"global_key": "bar",
				},
			},
		},
		{
			name: "network",
			in: map[string]string{
				"parent":     "foo",
				"global_key": "bar",
			},
			kind: "network",

			want: splitConfigs{
				Specific: map[string]string{
					"parent": "foo",
				},
				Global: map[string]string{
					"global_key": "bar",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitConfig(tc.in, tc.kind)

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
		{
			name: "unsupported kind",
			in:   map[string]string{},
			kind: "foobar",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Panics(t, func() {
				_ = splitConfig(tc.in, tc.kind)
			})
		})
	}
}
