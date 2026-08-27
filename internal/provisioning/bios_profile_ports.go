package provisioning

import (
	"context"
)

// BIOSProfilePort provides the BIOS profiles, that are available to be applied
// to a server before IncusOS is installed on it.
type BIOSProfilePort interface {
	// Resolve accumulates the BIOS profiles matching the BMC data of the
	// server, or returns nil when no profile matches.
	Resolve(ctx context.Context, server Server) (*BIOSProfileResolution, error)
}
