package cluster

import (
	"context"
	"crypto/x509"
	"log/slog"
	"maps"
	"slices"

	"github.com/cowsql/go-cowsql/cluster/db/transaction"
	incustls "github.com/lxc/incus/v7/shared/tls"

	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

func (m *membership) updateCertificateCacheFromLocal(ctx context.Context) {
	var certsByAddress map[string]api.Certificate

	err := transaction.ForceTx(ctx, m.localRepo, func(ctx context.Context, tx transaction.TX) error {
		var err error
		certsByAddress, err = m.localRepo.GetCertificates(ctx)

		return err
	})
	if err != nil {
		slog.ErrorContext(ctx, "Failed to retrieve server certificates from local cluster members", logger.Err(err))
		return
	}

	m.setCertificateCache(slices.Collect(maps.Values(certsByAddress)))
}

func (m *membership) updateCertificateCacheFromGlobal(ctx context.Context) {
	if m.globalRepo == nil {
		return
	}

	var certsByAddress map[string]api.Certificate

	err := transaction.ForceTx(ctx, m.globalRepo, func(ctx context.Context, tx transaction.TX) error {
		var err error
		certsByAddress, err = m.globalRepo.GetCertificates(ctx)

		return err
	})
	if err != nil {
		slog.ErrorContext(ctx, "Failed to retrieve server certificates from global cluster members", logger.Err(err))
		return
	}

	m.setCertificateCache(slices.Collect(maps.Values(certsByAddress)))

	// Ensure the persisted local cache is synced on this update.
	for address, cert := range certsByAddress {
		err = m.localRepo.SetRaftCertificateByAddress(ctx, cert, address)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to store the server certificate of a cluster member locally", slog.String("address", address), logger.Err(err))
		}
	}
}

// setCertificateCache updates the in-memory list of server certificates, used for intra-cluster communication.
func (m *membership) setCertificateCache(certs []api.Certificate) {
	m.clientCertMu.Lock()
	defer m.clientCertMu.Unlock()

	m.clientCertCache = map[string]x509.Certificate{}

	for _, cert := range certs {
		fp := cert.Fingerprint()
		if fp == "" {
			continue
		}

		m.clientCertCache[fp] = *cert.Certificate
	}
}

func (m *membership) getTrustedCerts() map[string]x509.Certificate {
	m.clientCertMu.RLock()
	defer m.clientCertMu.RUnlock()

	cache := make(map[string]x509.Certificate, len(m.clientCertCache))
	maps.Copy(cache, m.clientCertCache)

	return cache
}

// IsMemberCertificate reports if certificatePEM is the certificate of a cluster member.
func (m *membership) IsMemberCertificate(certificatePEM string) bool {
	cert, err := certificate.Decode([]byte(certificatePEM))
	if err != nil {
		return false
	}

	m.clientCertMu.RLock()
	defer m.clientCertMu.RUnlock()

	_, ok := m.clientCertCache[incustls.CertFingerprint(cert)]

	return ok
}
