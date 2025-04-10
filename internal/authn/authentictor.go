package authn

type Authenticator struct {
	trustedTLSClientCertFingerprints []string
}

func New(trustedTLSClientCertFingerprints []string) *Authenticator {
	return &Authenticator{
		trustedTLSClientCertFingerprints: trustedTLSClientCertFingerprints,
	}
}
