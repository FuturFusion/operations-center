package api

import "time"

// BMCRecordingFormatVersion is the version of the on disk recording format.
const BMCRecordingFormatVersion = 1

// BMCRecordingBodyEncodingBase64 marks a body in a BMC recording, that is
// base64 encoded, because it is not valid text.
const BMCRecordingBodyEncodingBase64 = "base64"

// BMCRecordingExchange is one HTTP exchange between Operations Center and a BMC.
type BMCRecordingExchange struct {
	// Sequence is the position of the exchange within the recording.
	// Example: 42
	Sequence int64 `json:"sequence" yaml:"sequence"`

	// Timestamp is the start of the attempt in RFC3339 format.
	// Example: 2026-10-27T08:15:00Z
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`

	// Duration of the attempt in nanoseconds.
	// Example: 250000000
	Duration time.Duration `json:"duration" yaml:"duration"`

	// DeploymentState is the state of the deployment step, that issued the request.
	// Example: apply-bios
	DeploymentState ServerDeploymentState `json:"deployment_state" yaml:"deployment_state"`

	// StepRetries is the retry count of the deployment step.
	// Example: 0
	StepRetries int `json:"step_retries" yaml:"step_retries"`

	// Attempt is the attempt within the retry transport. 0 is the first attempt.
	// Example: 0
	Attempt int `json:"attempt" yaml:"attempt"`

	// Method is the HTTP method of the request.
	// Example: PATCH
	Method string `json:"method" yaml:"method"`

	// URL is the full URL of the request.
	// Example: https://bmc.example.com/redfish/v1/Systems/1/Bios/Settings
	URL string `json:"url" yaml:"url"`

	// RequestHeaders holds the redacted headers of the request.
	// Example: {"Content-Type": ["application/json"]}
	RequestHeaders map[string][]string `json:"request_headers" yaml:"request_headers"`

	// RequestBody holds the body of the request, up to the size limit.
	// Example: {"Attributes": {"BootMode": "Uefi"}}
	RequestBody string `json:"request_body" yaml:"request_body"`

	// RequestBodyEncoding is empty for a text body and "base64" for a binary body.
	// Example: base64
	RequestBodyEncoding string `json:"request_body_encoding,omitempty" yaml:"request_body_encoding,omitempty"`

	// RequestSize is the real size of the request body in bytes.
	// Example: 38
	RequestSize int64 `json:"request_size" yaml:"request_size"`

	// RequestTruncated is true, if the request body exceeds the size limit.
	// Example: false
	RequestTruncated bool `json:"request_truncated,omitempty" yaml:"request_truncated,omitempty"`

	// Status is the HTTP status code of the response.
	// Example: 200
	Status int `json:"status" yaml:"status"`

	// ResponseHeaders holds the redacted headers of the response.
	// Example: {"Content-Type": ["application/json"]}
	ResponseHeaders map[string][]string `json:"response_headers" yaml:"response_headers"`

	// ResponseBody holds the body of the response, up to the size limit.
	// Example: {"@odata.id": "/redfish/v1/Systems/1/Bios/Settings"}
	ResponseBody string `json:"response_body" yaml:"response_body"`

	// ResponseBodyEncoding is empty for a text body and "base64" for a binary body.
	// Example: base64
	ResponseBodyEncoding string `json:"response_body_encoding,omitempty" yaml:"response_body_encoding,omitempty"`

	// ResponseSize is the real size of the response body in bytes.
	// Example: 56
	ResponseSize int64 `json:"response_size" yaml:"response_size"`

	// ResponseTruncated is true, if the response body exceeds the size limit.
	// Example: false
	ResponseTruncated bool `json:"response_truncated,omitempty" yaml:"response_truncated,omitempty"`

	// TransportError is set, if the round trip failed. The response fields stay
	// empty in this case.
	// Example: connection reset by peer
	TransportError string `json:"transport_error,omitempty" yaml:"transport_error,omitempty"`
}

// BMCRecordingManifest describes one recording.
type BMCRecordingManifest struct {
	// FormatVersion is the version of the recording format.
	// Example: 1
	FormatVersion int `json:"format_version" yaml:"format_version"`

	// Server is the name of the recorded server.
	// Example: server01
	Server string `json:"server" yaml:"server"`

	// RecordingID identifies the recording within the server.
	// Example: 20261027T081500Z
	RecordingID string `json:"recording_id" yaml:"recording_id"`

	// OperationsCenterVersion is the version of Operations Center, that made
	// the recording.
	// Example: 0.4.0
	OperationsCenterVersion string `json:"operations_center_version" yaml:"operations_center_version"`

	// StartedAt is the start of the deployment in RFC3339 format.
	// Example: 2026-10-27T08:15:00Z
	StartedAt time.Time `json:"started_at" yaml:"started_at"`

	// FinishedAt is the end of the deployment in RFC3339 format. It is empty
	// while the deployment runs.
	// Example: 2026-10-27T08:45:00Z
	FinishedAt time.Time `json:"finished_at,omitzero" yaml:"finished_at,omitempty"`

	// FinalState is the state, the deployment has ended in.
	// Example: completed
	FinalState ServerDeploymentState `json:"final_state,omitempty" yaml:"final_state,omitempty"`

	// Request is the deployment request.
	Request ServerDeploymentPost `json:"request" yaml:"request"`

	// BMCData is the data of the BMC at the start of the recording.
	BMCData BMCData `json:"bmc_data" yaml:"bmc_data"`

	// BMCEndpoint is the endpoint of the BMC.
	// Example: https://bmc.example.com
	BMCEndpoint string `json:"bmc_endpoint" yaml:"bmc_endpoint"`

	// MediaURLs holds the URLs of the media, that are attached to the server.
	// Example: ["https://operations-center.example.com/media/seed.iso"]
	MediaURLs []string `json:"media_urls,omitempty" yaml:"media_urls,omitempty"`

	// History holds the states the deployment has gone through.
	History []ServerDeploymentStep `json:"history,omitempty" yaml:"history,omitempty"`

	// Exchanges is the number of recorded exchanges.
	// Example: 1250
	Exchanges int64 `json:"exchanges" yaml:"exchanges"`

	// Bytes is the size of the recorded exchanges in bytes.
	// Example: 18350080
	Bytes int64 `json:"bytes" yaml:"bytes"`

	// TruncatedExchanges is the number of exchanges with a truncated body.
	// Example: 0
	TruncatedExchanges int64 `json:"truncated_exchanges" yaml:"truncated_exchanges"`
}
