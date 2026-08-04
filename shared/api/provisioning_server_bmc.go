package api

import (
	"encoding/json"
	"time"
)

// BMCLogEvent defines a log event from the BMC.
//
// swagger:model
type BMCLogEvent struct {
	// EntryCode holds the entry code for the log entry if the entry type is `SEL`.
	// Example: Informational
	EntryCode string `json:"entry_code" yaml:"entry_code"`

	// Message holds the actual message of the log event.
	// Example: This is a test log event.
	Message string `json:"message" yaml:"message"`

	// Severity of the log event. Possible values: OK, Warning, Critical
	// Example: OK
	Severity string `json:"severity" yaml:"severity"`

	// Timestamp of the log event in RFC3339 format.
	// Example: 2026-07-30T08:04:00Z
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`

	// EntryType holds the type of the entry. Possible values include (but not
	// limited to): SEL, Event, CXL, Oem.
	// Example: SEL
	EntryType string `json:"entry_type" yaml:"entry_type"`
}

// BMCDump holds the raw responses of the BMC (e.g. Redfish API), keyed by the
// endpoint name or path they were retrieved from.
//
// swagger:model
type BMCDump map[string]BMCDumpEntry

// BMCDumpEntry is the result of a single request against the BMC API.
//
// swagger:model
type BMCDumpEntry struct {
	// Response contains the raw JSON response returned by the BMC.
	Response json.RawMessage `json:"response,omitempty" yaml:"response,omitempty"`

	// Error contains the error details, if the request failed.
	Error *BMCDumpError `json:"error,omitempty" yaml:"error,omitempty"`

	// Trace contains additional dumped request and response details
	// (e.g. HTTP headers). Opaque field for human inspection only. Populated if
	// tracing is requested.
	Trace string `json:"trace,omitempty" yaml:"trace,omitempty"`
}

// BMCDumpError describes a failed request against the BMC API.
//
// swagger:model
type BMCDumpError struct {
	// Message contains the human readable error message.
	Message string `json:"message" yaml:"message"`

	// Code contains the error code, if any.
	Code string `json:"code,omitempty" yaml:"code,omitempty"`

	// StatusCode contains the http status code, if applicable.
	StatusCode int `json:"status_code,omitempty" yaml:"status_code,omitempty"`
}

// BMCVirtualMedia defines a single virtual media slot exposed by the BMC
// (system or manager), e.g. CD, DVD, floppy, or USB.
//
// swagger:model
type BMCVirtualMedia struct {
	// ID uniquely identifies this virtual media entry among all virtual media
	// reported by the BMC. It is derived from the service (e.g. "system") and ID.
	// Example: system:1
	ID string `json:"id" yaml:"id"`

	// Inserted reports, if a virtual media is currently inserted into this slot.
	Inserted bool `json:"inserted" yaml:"inserted"`

	// Image holds the URI of the media attached to the virtual media.
	Image string `json:"image" yaml:"image"`

	// ImageName holds the name of the inserted virtual media image.
	ImageName string `json:"image_name" yaml:"image_name"`

	// ConnectedVia holds the connection method of the virtual media (e.g. URI, Applet).
	ConnectedVia string `json:"connected_via" yaml:"connected_via"`

	// Status holds the reported health status of the virtual media.
	Status string `json:"status" yaml:"status"`

	// MediaTypes holds the media types supported by the virtual media.
	MediaTypes []string `json:"media_types" yaml:"media_types"`

	// TransferMethod describes how the image transfer occurs.
	TransferMethod string `json:"transfer_method" yaml:"transfer_method"`

	// TransferProtocolType holds the network protocol used with the image URI.
	TransferProtocolType string `json:"transfer_protocol_type" yaml:"transfer_protocol_type"`

	// WriteProtected reports, if the remote device media prevents writing to that media.
	WriteProtected bool `json:"write_protected" yaml:"write_protected"`
}
