package system

import "context"

type SystemService interface {
	UpdateCertificate(ctx context.Context, certificatePEM string, keyPEM string) error
}
