package api

import (
	"fmt"
	"strings"
	"time"
)

// Duration is a time.Duration, that is represented as a Go duration string,
// e.g. "15m" or "1h30m".
//
// swagger:type string
type Duration time.Duration

// String returns the duration without the trailing units, that are zero.
func (d Duration) String() string {
	s := time.Duration(d).String()

	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}

	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}

	return s
}

// MarshalText implements the encoding.TextMarshaler interface.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (d *Duration) UnmarshalText(text []byte) error {
	duration, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("%q is not a valid duration", string(text))
	}

	*d = Duration(duration)

	return nil
}
