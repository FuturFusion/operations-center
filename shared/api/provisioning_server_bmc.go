package api

import (
	"encoding/json"
	"fmt"
	"slices"
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

	// Allow contains the methods the BMC reported as allowed for this endpoint
	// in the "Allow" response header.
	Allow string `json:"allow,omitempty" yaml:"allow,omitempty"`

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

	// Error as reported by the API.
	Error string `json:"error" yaml:"error"`

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

// BMCBootProgress defines the last boot progress state reported by the BMC.
//
// swagger:model
type BMCBootProgress struct {
	// LastState holds the last boot progress state, e.g. "OSRunning" or
	// "SystemHardwareInitializationComplete". Empty if not reported.
	// Example: OSRunning
	LastState string `json:"last_state" yaml:"last_state"`

	// LastStateTime holds the time the last boot progress state was reached.
	// Example: 2024-11-12T16:15:00Z
	LastStateTime time.Time `json:"last_state_time" yaml:"last_state_time"`

	// LastBootTimeSeconds holds how long the last boot took.
	// Example: 315.5
	LastBootTimeSeconds float64 `json:"last_boot_time_seconds" yaml:"last_boot_time_seconds"`

	// OEMLastState holds the vendor specific state, set only when LastState is "OEM".
	OEMLastState string `json:"oem_last_state" yaml:"oem_last_state"`
}

// BMCRebootState is the outcome of the attempt to tell, if a server has
// rebooted, see BMCHasRebootedSince.
type BMCRebootState string

const (
	// BMCRebootStateUnknown means the BMC does not report any of the properties
	// required to tell, if the server has rebooted.
	BMCRebootStateUnknown BMCRebootState = "unknown"

	// BMCRebootStateNotRebooted means the BMC does report the properties and
	// they indicate, that the server has not rebooted.
	BMCRebootStateNotRebooted BMCRebootState = "not-rebooted"

	// BMCRebootStateRebooted means the server has rebooted.
	BMCRebootStateRebooted BMCRebootState = "rebooted"
)

func (s BMCRebootState) String() string {
	return string(s)
}

// bmcBootProgressOrder holds the boot progress states in the order, in which
// they are reached during a boot. States not part of this list (e.g. "None" or
// "OEM") carry no ordering information.
var bmcBootProgressOrder = []string{
	"PrimaryProcessorInitializationStarted",
	"BusInitializationStarted",
	"MemoryInitializationStarted",
	"SecondaryProcessorInitializationStarted",
	"PCIResourceConfigStarted",
	"SystemHardwareInitializationComplete",
	"SetupEntered",
	"OSBootStarted",
	"OSRunning",
}

// bmcBootProgressLateState is the first of the boot progress states, which are
// only reached once the firmware handed over to the operating system.
const bmcBootProgressLateState = "OSBootStarted"

// BMCHasRebootedSince reports, if the server has rebooted since the given time,
// by comparing a previous BMC data snapshot with the current one. Since neither
// of the required properties is supported by every BMC, the outcome is
// tri-state and callers need to handle BMCRebootStateUnknown by falling back to
// another signal.
func BMCHasRebootedSince(previous BMCData, current BMCData, since time.Time) BMCRebootState {
	state := BMCRebootStateUnknown

	// A last reset time moving backwards indicates a BMC clock reset, which
	// renders the property useless for the comparison.
	lastResetTimeIsSane := previous.ServerLastResetTime.IsZero() || !current.ServerLastResetTime.Before(previous.ServerLastResetTime)

	if !current.ServerLastResetTime.IsZero() && lastResetTimeIsSane {
		// The BMC clock is not necessarily in sync with our own, so a last
		// reset time after since only proves a reboot, if it also advanced
		// compared to the previous snapshot. Otherwise both snapshots just
		// describe the same reset, reported by a BMC clock running ahead.
		if current.ServerLastResetTime.After(since) && current.ServerLastResetTime.After(previous.ServerLastResetTime) {
			return BMCRebootStateRebooted
		}

		state = BMCRebootStateNotRebooted
	}

	previousProgress := previous.ServerBootProgress
	currentProgress := current.ServerBootProgress

	if bootProgressHasRegressed(previousProgress.LastState, currentProgress.LastState) {
		return BMCRebootStateRebooted
	}

	// Without the time of the last boot progress state, the absence of a
	// regression does not tell, if the server has rebooted. The same applies
	// to a last state time moving backwards, which indicates a BMC clock
	// reset and renders the property useless for the comparison. In both
	// cases, the state derived from the last reset time is kept.
	if currentProgress.LastStateTime.IsZero() || currentProgress.LastStateTime.Before(previousProgress.LastStateTime) {
		return state
	}

	// A boot progress state, that has been entered after the snapshot, only
	// proves a reboot, if the server did not simply move on within the boot it
	// was already in.
	if !currentProgress.LastStateTime.Before(since) &&
		currentProgress.LastStateTime.After(previousProgress.LastStateTime) &&
		bootProgressProvesReboot(previousProgress.LastState, currentProgress.LastState) {
		return BMCRebootStateRebooted
	}

	return BMCRebootStateNotRebooted
}

// bootProgressProvesReboot reports, if entering currentState after previousState
// can only be explained by the server having started over. A step forward is
// what the same boot progressing looks like, and a state, that can not be placed
// in the order of a boot, proves nothing either.
func bootProgressProvesReboot(previousState string, currentState string) bool {
	previousIndex := slices.Index(bmcBootProgressOrder, previousState)
	currentIndex := slices.Index(bmcBootProgressOrder, currentState)

	if previousIndex < 0 || currentIndex < 0 {
		return false
	}

	return currentIndex <= previousIndex
}

// bootProgressHasRegressed reports, if the boot progress fell back from a state
// only reached at the end of a boot to an earlier one, which is only possible
// if the server has rebooted in between.
func bootProgressHasRegressed(previousState string, currentState string) bool {
	previousIndex := slices.Index(bmcBootProgressOrder, previousState)
	currentIndex := slices.Index(bmcBootProgressOrder, currentState)
	lateIndex := slices.Index(bmcBootProgressOrder, bmcBootProgressLateState)

	if previousIndex < lateIndex || currentIndex < 0 {
		return false
	}

	return currentIndex < previousIndex
}

// ServerBMCApplyBIOSAttributesPost represents a request to apply a set of
// BIOS attributes to a server via its BMC.
//
// swagger:model
type ServerBMCApplyBIOSAttributesPost struct {
	// Attributes contains the BIOS attribute names and values to apply to the
	// server via BMC. The available attribute names and accepted value types
	// are BMC/BIOS vendor specific.
	Attributes map[string]any `json:"attributes" yaml:"attributes"`
}

// ServerBMCLocatePost represents a request to change the state of
// the location indicator LED of a server via its BMC.
//
// swagger:model
type ServerBMCLocatePost struct {
	// Active defines, if the location indicator LED should be turned on (true)
	// or off (false).
	// Example: true
	Active bool `json:"active" yaml:"active"`
}

// BIOSAttribute describes a single BIOS attribute known to the BMC.
//
// swagger:model
type BIOSAttribute struct {
	// Name holds the name of the BIOS attribute.
	// Example: NumaNodesPerSocket
	Name string `json:"name" yaml:"name"`

	// CurrentValue holds the current value of the BIOS attribute on the
	// server.
	// Example: 4
	CurrentValue any `json:"current_value" yaml:"current_value"`

	// Type holds the type of the BIOS attribute, e.g. Enumeration, String,
	// Integer, Boolean or Password.
	// Example: Enumeration
	Type string `json:"type" yaml:"type"`

	// LowerBound holds the lower limit for an Integer attribute, if declared
	// by the BMC.
	// Example: 0
	LowerBound *int64 `json:"lower_bound,omitempty" yaml:"lower_bound,omitempty"`

	// UpperBound holds the upper limit for an Integer attribute, if declared
	// by the BMC.
	// Example: 20
	UpperBound *int64 `json:"upper_bound,omitempty" yaml:"upper_bound,omitempty"`

	// MinLength holds the minimum character length for a String attribute,
	// if declared by the BMC.
	// Example: 4
	MinLength *int64 `json:"min_length,omitempty" yaml:"min_length,omitempty"`

	// MaxLength holds the maximum character length for a String attribute,
	// if declared by the BMC.
	// Example: 20
	MaxLength *int64 `json:"max_length,omitempty" yaml:"max_length,omitempty"`

	// AcceptableValues holds the values the BMC declares as acceptable for
	// the BIOS attribute. Empty if the attribute is not an enumeration.
	// Example: ["UserMode", "SetupMode"]
	AcceptableValues []string `json:"acceptable_values" yaml:"acceptable_values"`
}

// ServerBMCAttachMedia defines the request to attach installation media to a
// server via its BMC.
//
// swagger:model
type ServerBMCAttachMedia struct {
	// TokenUUID holds the UUID of the provisioning token that owns the seed.
	// Example: 8f6c3d1a-2b4e-4c9a-9f7d-1a2b3c4d5e6f
	TokenUUID string `json:"token_uuid" yaml:"token_uuid"`

	// Seed holds the name of the token seed used to generate the installation
	// media. The referenced token seed must be public.
	// Example: default
	Seed string `json:"seed" yaml:"seed"`

	// Type holds the type of image to generate. Possible values: iso, raw.
	// Example: iso
	Type string `json:"type" yaml:"type"`

	// Architecture holds the CPU architecture of the image to generate. Possible
	// values: x86_64, aarch64.
	// Example: x86_64
	Architecture string `json:"architecture" yaml:"architecture"`

	// Channel holds the channel the most recent update should be taken from to
	// generate the image. Optional, defaults to the configured default channel.
	// Example: stable
	Channel string `json:"channel" yaml:"channel"`

	// VirtualMediaID identifies the virtual media device the media is attached
	// to, using the "<service>:<bmc-id>" notation (e.g. "system:1" or
	// "manager:2") as reported in the BMC virtual media data.
	// Example: system:1
	VirtualMediaID string `json:"virtual_media_id" yaml:"virtual_media_id"`

	// SetBootDevice requests, that the virtual media is registered as the boot
	// device for the next boot of the server in addition to being attached.
	// Detaching the media restores the default boot configuration of the system.
	// Example: true
	SetBootDevice bool `json:"set_boot_device" yaml:"set_boot_device"`
}

// ServerBMCDetachMedia defines the request to detach installation media from a
// server via its BMC.
//
// swagger:model
type ServerBMCDetachMedia struct {
	// VirtualMediaID identifies the virtual media device the media is detached
	// from, using the "<service>:<bmc-id>" notation (e.g. "system:1" or
	// "manager:2") as reported in the BMC virtual media data.
	// Example: system:1
	VirtualMediaID string `json:"virtual_media_id" yaml:"virtual_media_id"`
}

// BMCTaskState describes the state of a task started via the BMC.
type BMCTaskState string

const (
	BMCTaskStateUnknown   BMCTaskState = "unknown"
	BMCTaskStateRunning   BMCTaskState = "running"
	BMCTaskStateCompleted BMCTaskState = "completed"
)

var bmcTaskStates = map[BMCTaskState]struct{}{
	BMCTaskStateUnknown:   {},
	BMCTaskStateRunning:   {},
	BMCTaskStateCompleted: {},
}

func (s BMCTaskState) String() string {
	return string(s)
}

func (s BMCTaskState) MarshalText() ([]byte, error) {
	return []byte(s), nil
}

func (s *BMCTaskState) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*s = BMCTaskStateUnknown
		return nil
	}

	_, ok := bmcTaskStates[BMCTaskState(text)]
	if !ok {
		return fmt.Errorf("%q is not a valid BMC task state", string(text))
	}

	*s = BMCTaskState(text)

	return nil
}
