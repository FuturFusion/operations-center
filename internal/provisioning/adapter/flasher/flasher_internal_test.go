package flasher

import (
	"crypto/tls"
	"net/http"
	"testing"

	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/flaky"
)

func Test_isSelfSigned(t *testing.T) {
	tests := []struct {
		name string
		cert tls.Certificate

		wantSelfSigned bool
	}{
		{
			name: "self signed",
			cert: func() tls.Certificate {
				certPEM, keyPEM, err := incustls.GenerateMemCert(false, false)
				require.NoError(t, err)

				cert, err := tls.X509KeyPair(certPEM, keyPEM)
				require.NoError(t, err)

				return cert
			}(),

			wantSelfSigned: true,
		},
		{
			name: "not self signed - linuxcontainers.org",
			cert: func() tls.Certificate {
				resp, err := http.Get("https://linuxcontainers.org")
				require.NoError(flaky.SkipOnFail(t, "calling external resources during test may fail"), err)

				defer resp.Body.Close()

				return tls.Certificate{
					Leaf: resp.TLS.PeerCertificates[0],
				}
			}(),

			wantSelfSigned: false,
		},
		{
			name: "not self signed - nil",
			cert: tls.Certificate{},

			wantSelfSigned: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isSelfSigned(tc.cert)
			require.Equal(t, tc.wantSelfSigned, got)
		})
	}
}
