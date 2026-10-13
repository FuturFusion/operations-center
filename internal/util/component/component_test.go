package component_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/component"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name          string
		componentName string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:          "single level",
			componentName: "api",
			assertErr:     require.NoError,
		},
		{
			name:          "multiple levels with underscore and digit",
			componentName: "provisioning.server_repo.v2",
			assertErr:     require.NoError,
		},
		{
			name:          "empty",
			componentName: "",
			assertErr:     require.Error,
		},
		{
			name:          "upper case",
			componentName: "API",
			assertErr:     require.Error,
		},
		{
			name:          "trailing dot",
			componentName: "api.",
			assertErr:     require.Error,
		},
		{
			name:          "leading dot",
			componentName: ".api",
			assertErr:     require.Error,
		},
		{
			name:          "empty level",
			componentName: "api..handler",
			assertErr:     require.Error,
		},
		{
			name:          "dash",
			componentName: "server-repo",
			assertErr:     require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.assertErr(t, component.ValidateName(tc.componentName))
		})
	}
}

func TestUnknownNames(t *testing.T) {
	tests := []struct {
		name  string
		known []string
		names []string

		want []string
	}{
		{
			name:  "no component known",
			known: nil,
			names: []string{"nowhere"},

			want: nil,
		},
		{
			name:  "known component and its parents",
			known: []string{"provisioning.server.repo"},
			names: []string{"provisioning", "provisioning.server", "provisioning.server.repo"},

			want: nil,
		},
		{
			name:  "unknown names in the given order",
			known: []string{"provisioning.server.repo", "api"},
			names: []string{"zzz", "api", "provisioning.server.repo.child", "provisioning.ser"},

			want: []string{"zzz", "provisioning.server.repo.child", "provisioning.ser"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, component.UnknownNames(tc.known, tc.names))
		})
	}
}

func TestLineage(t *testing.T) {
	tests := []struct {
		name          string
		componentName string

		want []string
	}{
		{
			name:          "empty",
			componentName: "",

			want: nil,
		},
		{
			name:          "single level",
			componentName: "api",

			want: []string{"api"},
		},
		{
			name:          "multiple levels",
			componentName: "provisioning.server.repo",

			want: []string{"provisioning.server.repo", "provisioning.server", "provisioning"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, slices.Collect(component.Lineage(tc.componentName)))
		})
	}
}
