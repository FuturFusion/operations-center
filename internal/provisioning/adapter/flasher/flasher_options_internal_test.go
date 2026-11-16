package flasher

import (
	"crypto/tls"
	"testing"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestNew_WithCacheUsage(t *testing.T) {
	certPEM, keyPEM, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	noSpace := func() (file.UsageInformation, error) {
		return file.UsageInformation{TotalSpaceBytes: 100, AvailableSpaceBytes: 1}, nil
	}

	tests := []struct {
		name    string
		options func(t *testing.T) []Option
	}{
		{
			name: "cache root first",
			options: func(t *testing.T) []Option {
				t.Helper()

				return []Option{WithCacheRoot(fsroot.Open(t, t.TempDir())), WithCacheUsage(noSpace)}
			},
		},
		{
			name: "cache usage first",
			options: func(t *testing.T) []Option {
				t.Helper()

				return []Option{WithCacheUsage(noSpace), WithCacheRoot(fsroot.Open(t, t.TempDir()))}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			flasher := New("https://operations-center.local:8443", cert, tc.options(t)...)

			err := flasher.cache.checkFreeSpace(1)

			errassert.DomainError(domain.ErrConstraintViolation, api.ErrorReasonInsufficientStorage)(t, err)
		})
	}
}
