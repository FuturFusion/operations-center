package api

// BMCLogEvent defines a log event from the BMC.
//
// swagger:model
type BMCLogEvent struct {
	// EntryCode holds the entry code for the log entry if the entry type is `SEL`.
	// Example: Informational
	EntryCode string

	// Message holds the actual message of the log event.
	// Example: This is a test log event.
	Message string

	// Severity of the log event. Possible values: OK, Warning, Critical
	// Example: OK
	Severity string

	// Timestamp of the log event in RFC3339 format.
	// Example: 2026-07-30T08:04:00Z
	Timestamp string

	// EntryType holds the type of the entry. Possible values include (but not
	// limited to): SEL, Event, CXL, Oem.
	// Example: SEL
	EntryType string
}
