package cluster

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"
	incustls "github.com/lxc/incus/v7/shared/tls"

	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// sameKey reports whether both certificates belong to the same key pair.
func sameKey(a *x509.Certificate, b *x509.Certificate) bool {
	key, ok := a.PublicKey.(interface{ Equal(crypto.PublicKey) bool })

	return ok && key.Equal(b.PublicKey)
}

// renewedClusterCertificate returns the PEM encoded chain presented by another member, if it renews the local cluster certificate:
// it must belong to the key this member holds and be newer. A certificate of another key can't be adopted, since the key is missing.
func renewedClusterCertificate(chain []*x509.Certificate, local *x509.Certificate) (chainPEM []byte, renewed bool) {
	if len(chain) == 0 || !sameKey(chain[0], local) || !chain[0].NotBefore.After(local.NotBefore) {
		return nil, false
	}

	for _, cert := range chain {
		chainPEM = append(chainPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})...)
	}

	return chainPEM, true
}

// remoteCertificates returns the certificate chain the server at address presents, replaced in tests.
var remoteCertificates = func(address string) ([]*x509.Certificate, error) {
	// The certificate is proven by the key this member holds, so the connection is not verified.
	return incustls.GetRemoteCertificates("https://"+address, "")
}

// SyncClusterCertificate adopts the cluster certificate another member presents, if it renews the one in varRoot for the same key.
// A member missing a renewal, since it was offline, catches up this way when it starts. It has to run before anything presents or
// pins the certificate, since the members only talk to a member presenting the very same certificate.
func SyncClusterCertificate(ctx context.Context, varRoot *os.Root, raft cowsqldb.Node, localAddress string) error {
	// A standalone server has no raft nodes.
	nodes, err := raft.GetRaftNodes(ctx)
	if err != nil || len(nodes) == 0 {
		return err
	}

	certPEM, err := varRoot.ReadFile("cluster.crt")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	local, err := certificate.Decode(certPEM)
	if err != nil {
		return fmt.Errorf("Failed to parse the cluster certificate: %w", err)
	}

	// The newest certificate of the key wins, the members might be in the middle of a renewal.
	var newest *x509.Certificate
	var newestPEM []byte
	for _, node := range nodes {
		if node.Address == localAddress {
			continue
		}

		chain, err := remoteCertificates(node.Address)
		if err != nil {
			slog.DebugContext(ctx, "Failed to get the cluster certificate of a member", slog.String("address", node.Address), logger.Err(err))
			continue
		}

		if !sameKey(chain[0], local) {
			slog.WarnContext(ctx, "Cluster member presents a cluster certificate of another key, this member can't follow it", slog.String("address", node.Address))
			continue
		}

		chainPEM, renewed := renewedClusterCertificate(chain, local)
		if renewed && (newest == nil || chain[0].NotBefore.After(newest.NotBefore)) {
			newest, newestPEM = chain[0], chainPEM
		}
	}

	if newest == nil {
		return nil
	}

	slog.InfoContext(ctx, "Adopting the renewed cluster certificate of the other members")

	return varRoot.WriteFile("cluster.crt", newestPEM, 0o644)
}
