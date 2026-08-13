package redfish

import (
	"errors"
	"fmt"
	"strings"

	"github.com/stmcginnis/gofish/schemas"
)

// redfishError renders the extended error information of a Redfish error
// response in a human readable form.
type redfishError struct {
	message string
	err     error
}

func (e redfishError) Error() string {
	return e.message
}

func (e redfishError) Unwrap() error {
	return e.err
}

// wrapRedfishError renders the Redfish error response carried by err, if there
// is one. Errors without a Redfish error response are returned unchanged.
func wrapRedfishError(err error) error {
	var redfishErr *schemas.Error
	if !errors.As(err, &redfishErr) {
		return err
	}

	return redfishError{
		message: formatRedfishError(redfishErr),
		err:     err,
	}
}

// formatRedfishError renders a Redfish error response, preferring the extended
// info entries, which carry the details, over the generic top level message.
func formatRedfishError(redfishErr *schemas.Error) string {
	details := make([]string, 0, len(redfishErr.ExtendedInfos))

	for _, extendedInfo := range redfishErr.ExtendedInfos {
		detail := formatRedfishMessage(schemas.Message(extendedInfo))
		if detail != "" {
			details = append(details, detail)
		}
	}

	// Not every BMC reports extended info. Fall back to the top level code and
	// message of the error response, which for responses not following the
	// Redfish error format at all holds the raw body.
	if len(details) == 0 {
		switch {
		case redfishErr.Code != "" && redfishErr.Message != "":
			details = append(details, redfishErr.Code+": "+redfishErr.Message)

		case redfishErr.Message != "":
			details = append(details, redfishErr.Message)

		default:
			details = append(details, "no error details reported")
		}
	}

	return fmt.Sprintf("BMC returned HTTP %d: %s", redfishErr.HTTPReturnedStatusCode, strings.Join(details, "; "))
}

// formatRedfishMessage renders a single message object of a Redfish error
// response. Only the message registry ID is guaranteed to be reported by a BMC,
// everything else is added if present.
func formatRedfishMessage(message schemas.Message) string {
	text := message.Message

	switch {
	case text == "" && message.MessageID == "":
		return ""

	case text == "":
		// Without a human readable message the registry ID and its arguments
		// are all there is to report.
		text = message.MessageID
		if len(message.MessageArgs) > 0 {
			text += " [" + strings.Join(message.MessageArgs, ", ") + "]"
		}

	case message.MessageID != "":
		text = message.MessageID + ": " + text
	}

	// RelatedProperties are RFC6901 JSON pointers into the request body, so
	// they name the exact properties the BMC took offense at.
	if len(message.RelatedProperties) > 0 {
		text += " (related properties: " + strings.Join(message.RelatedProperties, ", ") + ")"
	}

	if message.Resolution != "" {
		text += " Resolution: " + message.Resolution
	}

	return text
}
