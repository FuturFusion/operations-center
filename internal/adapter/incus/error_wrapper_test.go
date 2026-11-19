package incus_test

import (
	"net/http"
	"testing"

	incusclient "github.com/lxc/incus/v7/client"
	incusapi "github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/adapter/incus"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/middleware"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func TestNotFoundWrapper(t *testing.T) {
	t.Parallel()

	notFoundErr := incusapi.StatusErrorf(http.StatusNotFound, "not found")

	tests := []struct {
		name  string
		inErr error

		assertErr require.ErrorAssertionFunc
		wantIs    []error
		wantIsNot []error
	}{
		{
			name:  "nil error",
			inErr: nil,

			assertErr: require.NoError,
			wantIs:    nil,
			wantIsNot: nil,
		},
		{
			name:  "not found maps to domain.ErrNotFound",
			inErr: notFoundErr,

			assertErr: require.Error,
			wantIs:    []error{domain.ErrNotFound, notFoundErr},
			wantIsNot: nil,
		},
		{
			name:  "other status is not mapped",
			inErr: incusapi.StatusErrorf(http.StatusConflict, "conflict"),

			assertErr: require.Error,
			wantIs:    nil,
			wantIsNot: []error{domain.ErrNotFound},
		},
		{
			name:  "other error is not mapped",
			inErr: boom.Error,

			assertErr: require.Error,
			wantIs:    []error{boom.Error},
			wantIsNot: []error{domain.ErrNotFound},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			err := incus.NotFoundWrapper()(tc.inErr)

			// Assert
			tc.assertErr(t, err)

			for _, want := range tc.wantIs {
				require.ErrorIs(t, err, want, "expect error to match")
			}

			for _, wantNot := range tc.wantIsNot {
				require.NotErrorIs(t, err, wantNot, "expect error not to match")
			}
		})
	}
}

func TestInstanceServerWithNotFoundWrapper(t *testing.T) {
	t.Parallel()

	notFoundErr := incusapi.StatusErrorf(http.StatusNotFound, "not found")

	tests := []struct {
		name string
		call func(server provisioning.InstanceServer) error
	}{
		{
			name: "direct call maps not found",
			call: func(server provisioning.InstanceServer) error {
				return server.DeleteClusterMember("one", false)
			},
		},
		{
			name: "call on UseTarget result maps not found",
			call: func(server provisioning.InstanceServer) error {
				return server.UseTarget("target").DeleteClusterMember("one", false)
			},
		},
		{
			name: "call on UseProject result maps not found",
			call: func(server provisioning.InstanceServer) error {
				return server.UseProject("project").DeleteClusterMember("one", false)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			var base *adapterMock.InstanceServerMock
			base = &adapterMock.InstanceServerMock{
				UseTargetFunc:  func(name string) incusclient.InstanceServer { return base },
				UseProjectFunc: func(name string) incusclient.InstanceServer { return base },
				DeleteClusterMemberFunc: func(name string, force bool) error {
					return notFoundErr
				},
			}

			server := middleware.NewInstanceServerWithErrorWrapper(base, incus.NotFoundWrapper())

			// Run test
			err := tc.call(server)

			// Assert
			require.ErrorIs(t, err, notFoundErr, "expect the original error to be kept")
			require.ErrorIs(t, err, domain.ErrNotFound, "expect not found to be mapped")
		})
	}
}
