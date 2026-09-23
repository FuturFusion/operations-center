// Package violations holds one example per rule, so the checker is tested
// against code, which is type checked just like the code base itself.
package violations

import (
	"errors"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

// KindWrappedWithErrorf classifies the error the old way.
func KindWrappedWithErrorf(name string) error {
	return fmt.Errorf("Server %q not found: %w", name, domain.ErrNotFound)
}

// KindJoined classifies the error through errors.Join.
func KindJoined(cause error) error {
	return errors.Join(cause, domain.ErrConstraintViolation)
}

// KindWrapIgnored carries a directive, so it is not reported.
func KindWrapIgnored(name string) error {
	//domain-errors:ignore The message of this one is built elsewhere.
	return fmt.Errorf("Server %q not found: %w", name, domain.ErrNotFound)
}

// UnclassifiedBoundaryError reaches the user as an internal server error.
func UnclassifiedBoundaryError() response.Response {
	return response.SmartError(errors.New("The feature is not configured"))
}

// MessageNotCapitalized starts with a lower case letter.
func MessageNotCapitalized() error {
	return domain.NewErrorf(domain.ErrNotFound, "", "server not found")
}

// MessageTrailingPeriod ends with a period.
func MessageTrailingPeriod() error {
	return domain.NewErrorf(domain.ErrNotFound, "", "Server not found.")
}

// MessageWrapsError wraps the technical error into the message for the user.
func MessageWrapsError(cause error) error {
	return domain.NewErrorf(domain.ErrNotFound, "", "Server not found: %w", cause)
}

// MessageEmpty has nothing to tell the user.
func MessageEmpty() error {
	return domain.NewErrorf(domain.ErrNotFound, "", "")
}

// HintNotASentence does not end with a period.
func HintNotASentence() error {
	return domain.NewErrorf(domain.ErrNotFound, "", "Server not found").
		WithHintf("register the server first")
}

// HintClientSpecific names one particular client.
func HintClientSpecific() error {
	return domain.NewErrorf(domain.ErrNotFound, "", "Server not found").
		WithHintf("Run operations-center provisioning server add first.")
}

// ReasonNotDeclared passes a reason, which is not an api.ErrorReason.
func ReasonNotDeclared() error {
	return domain.NewErrorf(domain.ErrNotFound, "server_missing", "Server not found")
}

// DetailKeyNotSnakeCase names a detail the way the log does not.
func DetailKeyNotSnakeCase(name string) error {
	return domain.NewErrorf(domain.ErrNotFound, api.ErrorReasonNotFound, "Server %q not found", name).
		WithDetail("serverName", name)
}

// CauseReclassifiesError attaches a kind as the cause, which decides the status
// code instead of the kind of the error itself.
func CauseReclassifiesError() error {
	return domain.NewErrorf(domain.ErrNotAuthorized, "", "The token is not valid").
		WithCause(domain.ErrNotFound)
}

// BareKindEscapes hands the kind to the caller outside the infrastructure
// layer, so it reaches the user as "not found" without any context.
func BareKindEscapes() error {
	return domain.ErrNotFound
}
