package api

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// StatusErrorf returns a new StatusError containing the specified status and message.
func StatusErrorf(status int, format string, a ...any) StatusError {
	var msg string
	if len(a) > 0 {
		msg = fmt.Sprintf(format, a...)
	} else {
		msg = format
	}

	return StatusError{
		status: status,
		msg:    msg,
	}
}

// StatusError error type that contains an HTTP status code and message.
type StatusError struct {
	status int
	msg    string
}

// Error returns the error message or the http.StatusText() of the status code if message is empty.
func (e StatusError) Error() string {
	if e.msg != "" {
		return e.msg
	}

	return http.StatusText(e.status)
}

// Status returns the HTTP status code.
func (e StatusError) Status() int {
	return e.status
}

// StatusErrorMatch checks if err was caused by StatusError. Can optionally also check whether the StatusError's
// status code matches one of the supplied status codes in matchStatus.
// Returns the matched StatusError status code and true if match criteria are met, otherwise false.
func StatusErrorMatch(err error, matchStatusCodes ...int) (int, bool) {
	var statusErr StatusError

	if errors.As(err, &statusErr) {
		statusCode := statusErr.Status()

		if len(matchStatusCodes) <= 0 {
			return statusCode, true
		}

		if slices.Contains(matchStatusCodes, statusCode) {
			return statusCode, true
		}
	}

	return -1, false
}

// StatusErrorCheck returns whether or not err was caused by a StatusError and if it matches one of the
// optional status codes.
func StatusErrorCheck(err error, matchStatusCodes ...int) bool {
	_, found := StatusErrorMatch(err, matchStatusCodes...)
	return found
}

type notIncusOSError struct{}

var NotIncusOSError = notIncusOSError{}

const notIncusOSErrorMessage = "System isn't running IncusOS"

func (notIncusOSError) Error() string {
	return notIncusOSErrorMessage
}

func (notIncusOSError) Is(target error) bool {
	return target == NotIncusOSError
}

// AsNotIncusOSError returns a NotIncusOSError, if the passed error matches the
// message of NotIncusOSError. Otherwise the passed error is returned
// unaltered.
func AsNotIncusOSError(err error) error {
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), notIncusOSErrorMessage) {
		return notIncusOSError{}
	}

	return err
}

// RequestIDHeader is the HTTP header, which carries the ID of a request. The ID
// is reported in the log records of the request and in the metadata of error
// responses, which allows to correlate an error reported to the user with the
// log.
const RequestIDHeader = "X-Request-Id"

// ErrorReason is a stable, machine readable identifier for the cause of an
// error, which allows clients to react to it.
type ErrorReason string

// Generic error reasons, which are derived from the kind of an error.
const (
	ErrorReasonInternal              ErrorReason = "internal"
	ErrorReasonInvalidArgument       ErrorReason = "invalid_argument"
	ErrorReasonNotFound              ErrorReason = "not_found"
	ErrorReasonConstraintViolation   ErrorReason = "constraint_violation"
	ErrorReasonOperationNotPermitted ErrorReason = "operation_not_permitted"
	ErrorReasonUnauthenticated       ErrorReason = "unauthenticated"
	ErrorReasonForbidden             ErrorReason = "forbidden"
	ErrorReasonPreconditionFailed    ErrorReason = "precondition_failed"
	ErrorReasonNotImplemented        ErrorReason = "not_implemented"
	ErrorReasonUnavailable           ErrorReason = "unavailable"
	ErrorReasonUpstream              ErrorReason = "upstream"
	ErrorReasonInsufficientStorage   ErrorReason = "insufficient_storage"
)

// Specific error reasons, which identify a concrete error condition.
const (
	ErrorReasonClusterTooSmall            ErrorReason = "cluster_too_small"
	ErrorReasonImagesOnlyOnRemovedServers ErrorReason = "images_only_on_removed_servers"
	ErrorReasonServerHasCustomVolumes     ErrorReason = "server_has_custom_volumes"
	ErrorReasonServerHasInstances         ErrorReason = "server_has_instances"
	ErrorReasonServerIsClusterMember      ErrorReason = "server_is_cluster_member"
	ErrorReasonServerNotClusterMember     ErrorReason = "server_not_cluster_member"
	ErrorReasonServerNotEvacuated         ErrorReason = "server_not_evacuated"
)

// ErrorMetadata is reported in the metadata of an error response.
//
// swagger:model
type ErrorMetadata struct {
	// Reason is a stable, machine readable identifier for the cause of the error.
	// Example: not_found
	Reason ErrorReason `json:"reason" yaml:"reason"`

	// Details contains the dynamic values of the error message in machine
	// readable form.
	// Example: {"server": "server01"}
	Details map[string]string `json:"details,omitempty" yaml:"details,omitempty"`

	// RequestID is the ID of the request, which caused the error. It allows to
	// find the corresponding records in the log of the server.
	// Example: 550e8400-e29b-41d4-a716-446655440000
	RequestID string `json:"request_id,omitempty" yaml:"request_id,omitempty"`
}
