package api

// StatusCode represents a valid operation and container status.
type StatusCode int

// Status codes.
const (
	Success StatusCode = 200

	Failure StatusCode = 400
)

// StatusCodeNames associates a status code to its name.
var StatusCodeNames = map[StatusCode]string{
	Success: "Success",
	Failure: "Failure",
}

// String returns a suitable string representation for the status code.
func (o StatusCode) String() string {
	return StatusCodeNames[o]
}
