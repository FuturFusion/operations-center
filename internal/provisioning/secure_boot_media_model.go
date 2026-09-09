package provisioning

import (
	"io"
	"time"
)

// SecureBootCertificates holds the PEM encoded certificates to be enrolled into
// the UEFI key databases of a server, in the order they are enrolled in.
type SecureBootCertificates struct {
	PK  string
	KEK []string
	DB  []string
	DBX []string

	// PKUpdate is the signed update enrolling PK, as IncusOS ships it.
	PKUpdate []byte
}

func (c SecureBootCertificates) IsEmpty() bool {
	return c.PK == "" && len(c.KEK) == 0 && len(c.DB) == 0 && len(c.DBX) == 0
}

// SecureBootMediaImage is one generated secure boot enrollment media, ready to
// be served.
type SecureBootMediaImage struct {
	// Content holds the image itself. It is the callers duty to close it.
	Content  io.ReadSeekCloser
	Filename string
	Size     int64
	ModTime  time.Time
}
