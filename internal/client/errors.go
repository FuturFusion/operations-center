package client

import (
	"net/http"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

// ServerError is an error response received from the Operations Center server.
type ServerError struct {
	// StatusCode is the HTTP status code of the response.
	StatusCode int

	// Message is the error message reported by the server.
	Message string

	// Reason identifies the cause of the error, see api.ErrorReason.
	Reason api.ErrorReason

	// Hint tells the user how to resolve the error, if the server reports one.
	Hint string

	// Details contains the dynamic values of the message in machine readable form.
	Details map[string]string

	// RequestID identifies the request, which caused the error. It allows to
	// find the corresponding records in the log of the server.
	RequestID string
}

// Error returns the message reported by the server.
func (e *ServerError) Error() string {
	if e.Message == "" {
		return http.StatusText(e.StatusCode)
	}

	return e.Message
}

// Is maps the status code of the response to the matching domain error, so
// callers can handle the error without knowledge about HTTP status codes.
func (e *ServerError) Is(target error) bool {
	switch e.StatusCode {
	case http.StatusNotFound:
		return target == domain.ErrNotFound

	case http.StatusUnauthorized:
		return target == domain.ErrNotAuthenticated

	case http.StatusForbidden:
		return target == domain.ErrNotAuthorized
	}

	return false
}

// As reports the error as api.StatusError, so the status code of the response
// can be matched using api.StatusErrorMatch.
func (e *ServerError) As(target any) bool {
	statusErr, ok := target.(*api.StatusError)
	if !ok {
		return false
	}

	*statusErr = api.StatusErrorf(e.StatusCode, "%s", e.Error())

	return true
}
