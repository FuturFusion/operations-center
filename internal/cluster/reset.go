package cluster

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	securitytls "github.com/FuturFusion/operations-center/internal/security/tls"
)

// resetPendingFilename marks a member reset to a standalone server, whose database is still to be removed.
const resetPendingFilename = "reset.pending"

// CompletePendingReset finishes the reset of this member to a standalone server, if the daemon died before its shutdown removed the database.
// It returns true if a reset was completed.
func CompletePendingReset(ctx context.Context, varRoot *os.Root) (bool, error) {
	_, err := varRoot.Stat(resetPendingFilename)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	slog.InfoContext(ctx, "Completing the reset to a standalone server")

	errs := []error{varRoot.RemoveAll("database"), memberfiles.DeleteStores(varRoot)}

	for _, suffix := range []string{"ca", "crt", "key"} {
		err := varRoot.Remove("cluster." + suffix)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	// The client keypair of the cluster is replaced, even if the reset got that far already.
	_, _, err = securitytls.GenerateClientKeypair(varRoot)
	errs = append(errs, err)

	err = errors.Join(errs...)
	if err != nil {
		return true, fmt.Errorf("Failed to complete the reset to a standalone server: %w", err)
	}

	return true, varRoot.Remove(resetPendingFilename)
}
