package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
)

func TestLifecycleSource_String(t *testing.T) {
	tests := []struct {
		name string

		want string
	}{
		{
			name: "success complete",

			want: "/parent-type/parent-name/type/name?project=project",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.LifecycleSource{
				ProjectName: "project",
				ParentType:  "parent-type",
				ParentName:  "parent-name",
				Name:        "name",
				Type:        "type",
			}.String()

			require.Equal(t, tc.want, got)
		})
	}
}
