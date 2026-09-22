package response

import (
	"database/sql"
	"errors"
	"net/http"
	"os"

	incusapi "github.com/lxc/incus/v7/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

var httpResponseErrors = map[int][]error{
	http.StatusNotFound:  {os.ErrNotExist, sql.ErrNoRows},
	http.StatusForbidden: {os.ErrPermission},
}

// SmartError returns the right error response based on err.
//
// The status code and the reason are derived from the kind of the error. The
// message reported to the user is the message of err without the technical
// details, see domain.UserMessage.
func SmartError(err error) Response {
	if err == nil {
		return EmptySyncResponse
	}

	for statusCode, checkErrs := range httpResponseErrors {
		for _, checkErr := range checkErrs {
			// An error, which has not been wrapped, carries no information for
			// the user, so it is replaced by the generic HTTP status text.
			if err == checkErr { //nolint:errorlint // Intentional comparison, a wrapped error is handled below.
				return &errorResponse{
					code:   statusCode,
					msg:    http.StatusText(statusCode),
					reason: reasonForStatusCode(statusCode),
					err:    err,
				}
			}
		}
	}

	statusCode, reason := statusCodeAndReason(err)

	var details map[string]string

	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		if domainErr.Reason() != "" {
			reason = api.ErrorReason(domainErr.Reason())
		}

		details = domainErr.Details()
	}

	return &errorResponse{
		code:    statusCode,
		msg:     domain.UserMessage(err),
		reason:  reason,
		details: details,
		err:     err,
	}
}

// statusCodeAndReason returns the HTTP status code and the generic reason for
// the kind of the given error.
func statusCodeAndReason(err error) (int, api.ErrorReason) {
	var validationErr domain.ErrValidation

	switch {
	case errors.As(err, &validationErr):
		return http.StatusBadRequest, api.ErrorReasonInvalidArgument

	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, api.ErrorReasonNotFound

	case errors.Is(err, domain.ErrConstraintViolation):
		return http.StatusBadRequest, api.ErrorReasonConstraintViolation

	case errors.Is(err, domain.ErrOperationNotPermitted):
		return http.StatusBadRequest, api.ErrorReasonOperationNotPermitted

	case errors.Is(err, domain.ErrNotAuthenticated):
		return http.StatusUnauthorized, api.ErrorReasonUnauthenticated

	case errors.Is(err, domain.ErrNotAuthorized):
		return http.StatusForbidden, api.ErrorReasonForbidden
	}

	statusCode, found := api.StatusErrorMatch(err)
	if found {
		return statusCode, reasonForStatusCode(statusCode)
	}

	// A retryable error is caused by a temporary condition, so the client is
	// told to try again later.
	if domain.IsRetryableError(err) {
		return http.StatusServiceUnavailable, api.ErrorReasonUnavailable
	}

	// An error reported by an Incus server is not an error of the Operations
	// Center API itself, so it is reported as error of the upstream server.
	_, found = incusapi.StatusErrorMatch(err)
	if found {
		return http.StatusBadGateway, api.ErrorReasonUpstream
	}

	for statusCode, checkErrs := range httpResponseErrors {
		for _, checkErr := range checkErrs {
			if errors.Is(err, checkErr) {
				return statusCode, reasonForStatusCode(statusCode)
			}
		}
	}

	return http.StatusInternalServerError, api.ErrorReasonInternal
}

// reasonForStatusCode returns the generic reason for the given HTTP status code.
func reasonForStatusCode(statusCode int) api.ErrorReason {
	switch statusCode {
	case http.StatusBadRequest:
		return api.ErrorReasonInvalidArgument

	case http.StatusUnauthorized:
		return api.ErrorReasonUnauthenticated

	case http.StatusForbidden:
		return api.ErrorReasonForbidden

	case http.StatusNotFound:
		return api.ErrorReasonNotFound

	case http.StatusConflict:
		return api.ErrorReasonConstraintViolation

	case http.StatusPreconditionFailed:
		return api.ErrorReasonPreconditionFailed

	case http.StatusNotImplemented:
		return api.ErrorReasonNotImplemented

	case http.StatusBadGateway:
		return api.ErrorReasonUpstream

	case http.StatusServiceUnavailable:
		return api.ErrorReasonUnavailable

	case http.StatusInsufficientStorage:
		return api.ErrorReasonInsufficientStorage

	default:
		return api.ErrorReasonInternal
	}
}

// IsNotFoundError returns true if the error is considered a Not Found error.
func IsNotFoundError(err error) bool {
	if api.StatusErrorCheck(err, http.StatusNotFound) {
		return true
	}

	for _, checkErr := range httpResponseErrors[http.StatusNotFound] {
		if errors.Is(err, checkErr) {
			return true
		}
	}

	return false
}
