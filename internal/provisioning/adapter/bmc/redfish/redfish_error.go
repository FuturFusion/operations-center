package redfish

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/stmcginnis/gofish/schemas"

	"github.com/FuturFusion/operations-center/shared/api"
)

// redfishError renders the extended error information of a Redfish error
// response in a human readable form.
type redfishError struct {
	message    string
	statusCode int
	err        error
}

func (e redfishError) Error() string {
	return e.message
}

// Unwrap exposes the original error and, for a request the BMC turned down,
// the status the API answers with.
func (e redfishError) Unwrap() []error {
	if e.statusCode >= 400 && e.statusCode < 500 {
		return []error{e.err, api.StatusErrorf(http.StatusBadRequest, "%s", e.message)}
	}

	return []error{e.err}
}

// wrapRedfishError renders the Redfish error response carried by err, if there
// is one. Errors without a Redfish error response are returned unchanged.
func wrapRedfishError(err error) error {
	return wrapRedfishErrorWithRegistry(err, nil)
}

// wrapRedfishErrorWithRegistry renders the Redfish error response carried by
// err, expanding messages the BMC reported by their registry ID alone.
func wrapRedfishErrorWithRegistry(err error, registry *messageRegistry) error {
	var wrapped redfishError
	if errors.As(err, &wrapped) {
		return err
	}

	var redfishErr *schemas.Error
	if !errors.As(err, &redfishErr) {
		return err
	}

	return redfishError{
		message:    formatRedfishError(redfishErr, registry),
		statusCode: redfishErr.HTTPReturnedStatusCode,
		err:        err,
	}
}

// redfishRequestError renders the Redfish error response carried by err and
// prefixes it with the request which triggered it.
func redfishRequestError(err error, registry *messageRegistry, method string, uri string, payload any) error {
	if err == nil {
		return nil
	}

	request := method + " " + uri

	body, marshalErr := json.Marshal(payload)
	if marshalErr == nil && payload != nil {
		request += " " + string(body)
	}

	return fmt.Errorf("%s: %w", request, wrapRedfishErrorWithRegistry(err, registry))
}

// formatRedfishError renders a Redfish error response, preferring the extended
// info entries, which carry the details, over the generic top level message.
//
// The registry is optional and only consulted for messages the BMC reported by
// their registry ID alone.
func formatRedfishError(redfishErr *schemas.Error, registry *messageRegistry) string {
	details := make([]string, 0, len(redfishErr.ExtendedInfos))

	for _, extendedInfo := range redfishErr.ExtendedInfos {
		detail := formatRedfishMessage(schemas.Message(extendedInfo), registry)
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
// everything else is added if present or, for the message text and the
// resolution, looked up in the message registry of the BMC.
func formatRedfishMessage(message schemas.Message, registry *messageRegistry) string {
	if message.Message == "" && message.MessageID == "" {
		return ""
	}

	text := message.Message
	resolution := message.Resolution

	if text == "" {
		text, resolution = expandFromRegistry(message, registry)
	}

	switch {
	case text == "":
		// Neither the BMC nor its registry has anything to say about the
		// message, so the registry ID and its arguments are all there is to
		// report.
		text = message.MessageID
		if len(message.MessageArgs) > 0 {
			text += " [" + strings.Join(message.MessageArgs, ", ") + "]"
		}

	case message.MessageID != "":
		text = message.MessageID + ": " + text
	}

	severity := messageSeverity(message)
	if severity != "" {
		text += " (severity: " + severity + ")"
	}

	// RelatedProperties are RFC6901 JSON pointers into the request body, so
	// they name the exact properties the BMC took offense at.
	if len(message.RelatedProperties) > 0 {
		text += " (related properties: " + strings.Join(message.RelatedProperties, ", ") + ")"
	}

	if resolution != "" {
		text += " Resolution: " + resolution
	}

	return text
}

// messageSeverity returns how severe the BMC considers the message.
//
// Severity was superseded by MessageSeverity in Message v1.1.0, BMCs report
// either one or both.
func messageSeverity(message schemas.Message) string {
	if message.MessageSeverity != "" {
		return string(message.MessageSeverity)
	}

	return message.Severity //nolint:staticcheck // Reported by BMCs implementing Message before v1.1.0.
}

// expandFromRegistry looks the message up in the message registry of the BMC
// and fills its arguments in.
func expandFromRegistry(message schemas.Message, registry *messageRegistry) (text string, resolution string) {
	entry := registry.lookup(message.MessageID)
	if entry == nil {
		return "", message.Resolution
	}

	resolution = message.Resolution
	if resolution == "" {
		resolution = entry.Resolution
	}

	return expandMessageArgs(entry.Message, message.MessageArgs), resolution
}

func isParameterMissing(err error, parameter string) bool {
	return redfishErrorHasMessageID(err, "ActionParameterMissing", "PropertyMissing", "CreateFailedMissingReqProperties", "GeneralError") &&
		redfishErrorMentions(err, parameter)
}

func isPropertyRejected(err error, property string) bool {
	return redfishErrorHasMessageID(err, "PropertyUnknown", "PropertyNotWritable", "PropertyUnknownOrUnwritable") &&
		redfishErrorMentions(err, property)
}

func isValueRejected(err error, property string, value string) bool {
	return redfishErrorHasMessageID(err, "PropertyValueNotInList", "PropertyValueNotSupported", "PropertyValueTypeError", "PropertyValueError", "PropertyValueOutOfRange", "PropertyValueConflict", "PropertyNotWritable", "GeneralError") &&
		(redfishErrorMentions(err, property) || redfishErrorMentions(err, value))
}

func isPreconditionRejected(err error) bool {
	var redfishErr *schemas.Error
	if !errors.As(err, &redfishErr) {
		return false
	}

	if redfishErr.HTTPReturnedStatusCode == http.StatusPreconditionFailed ||
		redfishErr.HTTPReturnedStatusCode == http.StatusPreconditionRequired {
		return true
	}

	if redfishErrorHasMessageID(err, "PreconditionFailed", "PreconditionRequired") {
		return true
	}

	// BMCs rejecting a header they do not understand answer with a plain client
	// error naming it.
	return redfishErrorHasMessageID(err, "HeaderMissing", "HeaderInvalid", "GeneralError") &&
		redfishErrorMentions(err, "If-Match")
}

func redfishErrorHasMessageID(err error, ids ...string) bool {
	var redfishErr *schemas.Error
	if !errors.As(err, &redfishErr) {
		return false
	}

	messageIDs := make([]string, 0, len(redfishErr.ExtendedInfos)+1)
	messageIDs = append(messageIDs, redfishErr.Code)

	for _, extendedInfo := range redfishErr.ExtendedInfos {
		messageIDs = append(messageIDs, extendedInfo.MessageID)
	}

	for _, messageID := range messageIDs {
		_, id, found := lastCut(messageID, ".")
		if found && slices.Contains(ids, id) {
			return true
		}
	}

	return false
}

func redfishErrorMentions(err error, name string) bool {
	var redfishErr *schemas.Error
	if !errors.As(err, &redfishErr) {
		return false
	}

	if strings.Contains(redfishErr.Message, name) {
		return true
	}

	for _, extendedInfo := range redfishErr.ExtendedInfos {
		if slices.Contains(extendedInfo.MessageArgs, name) || strings.Contains(extendedInfo.Message, name) {
			return true
		}

		for _, relatedProperty := range extendedInfo.RelatedProperties {
			if strings.TrimPrefix(relatedProperty, "#") == "/"+name {
				return true
			}
		}
	}

	return false
}

func lastCut(s string, sep string) (before string, after string, found bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}

	return s[:i], s[i+len(sep):], true
}
