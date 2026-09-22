package domain

import (
	"errors"
	"fmt"
	"maps"
	"strings"
)

// Error is an error with a message, which is meant to be reported to the user.
//
// The message is self-contained, it describes what went wrong and, if possible,
// how to resolve it, without repeating context added by other layers. The cause
// carries the technical details, which are only relevant for debugging and are
// therefore only reported in the log.
type Error struct {
	kind    error
	reason  string
	message string
	details map[string]string
	cause   error
}

// NewErrorf returns an Error of the given kind with a message for the user.
//
// The kind classifies the error, e.g. ErrNotFound, and is matched by errors.Is.
// The reason is an optional stable identifier for the concrete error condition,
// e.g. api.ErrorReasonServerNotEvacuated, which allows clients to react to it.
// If the reason is empty, the client falls back to the reason derived from the
// kind.
func NewErrorf[Reason ~string](kind error, reason Reason, format string, a ...any) *Error {
	return &Error{
		kind:    kind,
		reason:  string(reason),
		message: fmt.Sprintf(format, a...),
	}
}

// WithCause returns a copy of the error with the given cause attached. The
// cause is reported in the log but never to the user.
func (e *Error) WithCause(cause error) *Error {
	err := e.clone()
	err.cause = cause

	return err
}

// WithDetail returns a copy of the error with the given detail added. Details
// carry the dynamic values of the message in machine readable form.
func (e *Error) WithDetail(key string, value string) *Error {
	err := e.clone()
	if err.details == nil {
		err.details = map[string]string{}
	}

	err.details[key] = value

	return err
}

func (e *Error) clone() *Error {
	err := *e
	err.details = maps.Clone(e.details)

	return &err
}

// Error returns the message together with the cause, so the log contains the
// full detail. Use Message to get the message for the user.
func (e *Error) Error() string {
	if e.cause == nil {
		return e.message
	}

	return e.message + ": " + e.cause.Error()
}

// Message returns the message for the user.
func (e *Error) Message() string {
	return e.message
}

// Kind returns the kind of the error, e.g. ErrNotFound.
func (e *Error) Kind() error {
	return e.kind
}

// Reason returns the reason of the error or an empty string, if no reason is set.
func (e *Error) Reason() string {
	return e.reason
}

// Details returns the details of the error.
func (e *Error) Details() map[string]string {
	return maps.Clone(e.details)
}

// Is reports whether the error is of the given kind.
func (e *Error) Is(target error) bool {
	return e.kind != nil && e.kind == target
}

// As reports whether the kind of the error matches the given target, e.g. an
// ErrValidation for an error of kind ErrInvalidArgument.
func (e *Error) As(target any) bool {
	if e.kind == nil {
		return false
	}

	return errors.As(e.kind, target)
}

// Unwrap returns the cause of the error.
func (e *Error) Unwrap() error {
	return e.cause
}

// userMessageKinds are the error kinds, which are wrapped using %w and
// therefore show up as suffix of the error message.
var userMessageKinds = []error{
	ErrNotFound,
	ErrConstraintViolation,
	ErrOperationNotPermitted,
	ErrNotAuthenticated,
	ErrNotAuthorized,
	ErrTerminal,
}

// UserMessage returns the message of err, which is meant to be reported to the
// user.
//
// For an Error, this is its message, the cause is left out. For any other error
// it is the complete error message, cleaned up from the kind, which is appended
// to it by wrapping, e.g. "Operation not permitted".
func UserMessage(err error) string {
	if err == nil {
		return ""
	}

	var domainErr *Error
	if errors.As(err, &domainErr) {
		return domainErr.Message()
	}

	message := err.Error()

	for _, kind := range userMessageKinds {
		suffix := ": " + kind.Error()
		if errors.Is(err, kind) && strings.HasSuffix(message, suffix) {
			message = strings.TrimSuffix(message, suffix)
		}
	}

	return message
}
