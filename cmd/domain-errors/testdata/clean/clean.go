// Package clean holds errors, which follow the conventions, so a rule, which
// is too eager, is caught as well.
package clean

import (
	"errors"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

// WellFormed carries everything a user facing error is meant to carry.
func WellFormed(name string, cluster string) error {
	return domain.NewErrorf(domain.ErrOperationNotPermitted, api.ErrorReasonServerIsClusterMember,
		"Server %q is a member of cluster %q and can not be deleted", name, cluster).
		WithHintf("Remove the server from the cluster first.").
		WithDetail("server", name).
		WithDetail("cluster", cluster)
}

// FailedStepIsAUserMessage names the operation the user asked for, which is
// what the user wants to read.
func FailedStepIsAUserMessage(name string, attempts int) error {
	return domain.NewErrorf(domain.ErrTerminal, "", "Failed to update server %q in %d attempts", name, attempts).
		WithHintf("Resolve the reported problem and start the update again.")
}

// TestingAKindIsNotClassifying recognises an error rather than building one.
func TestingAKindIsNotClassifying(err error) bool {
	return errors.Is(err, domain.ErrNotFound)
}

// WrappingForTheLogIsFine adds context, which only the log sees.
func WrappingForTheLogIsFine(name string, err error) error {
	return fmt.Errorf("Failed to get server %q by name: %w", name, err)
}

// ForcedStatusCodeIsFine reports the message to the user with a status code of
// its own, so the error does not need a kind.
func ForcedStatusCodeIsFine(archive string) response.Response {
	return response.BadRequest(fmt.Errorf("Archive type %q not supported", archive))
}

// WrappedBoundaryErrorIsFine inherits the kind of the error it wraps.
func WrappedBoundaryErrorIsFine(err error) response.Response {
	return response.SmartError(fmt.Errorf("Failed to get the servers: %w", err))
}

// DynamicHintIsCheckedWhereItIsWritten passes the sentence through.
func DynamicHintIsCheckedWhereItIsWritten(hint string) error {
	return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "The operation is not permitted").
		WithHintf("%s", hint)
}

// DynamicReasonIsResolvedByTheCaller carries the api.ErrorReason type.
func DynamicReasonIsResolvedByTheCaller(reason api.ErrorReason, format string, a ...any) error {
	return domain.NewErrorf(domain.ErrOperationNotPermitted, reason, format, a...)
}
