package flasher

import (
	"crypto/tls"
	"crypto/x509"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/certs"
)

func TestIsCertChainTrusted(t *testing.T) {
	rootPEM, intermediatePEM, leafPEM, leafKeyPEM := certs.GenerateChain(t)

	chain, err := tls.X509KeyPair(append(append([]byte{}, leafPEM...), intermediatePEM...), leafKeyPEM)
	require.NoError(t, err)

	leafOnly, err := tls.X509KeyPair(leafPEM, leafKeyPEM)
	require.NoError(t, err)

	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(rootPEM))

	brokenChain := chain
	brokenChain.Certificate = [][]byte{chain.Certificate[0], []byte("not a certificate")}

	tests := []struct {
		name string
		cert tls.Certificate
		pool *x509.CertPool

		want bool
	}{
		{
			name: "trusted - leaf with intermediate",
			cert: chain,
			pool: roots,

			want: true,
		},
		{
			name: "not trusted - intermediate missing",
			cert: leafOnly,
			pool: roots,

			want: false,
		},
		{
			name: "not trusted - root CA unknown",
			cert: chain,
			pool: x509.NewCertPool(),

			want: false,
		},
		{
			name: "not trusted - intermediate can not be parsed",
			cert: brokenChain,
			pool: roots,

			want: false,
		},
		{
			name: "not trusted - no certificate",
			cert: tls.Certificate{},
			pool: roots,

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isCertChainTrusted(tc.cert, tc.pool))
		})
	}
}
