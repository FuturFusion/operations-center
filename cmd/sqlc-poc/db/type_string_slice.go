package db

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

type StringSlice []string

// MarshalText implements the encoding.TextMarshaler interface.
func (s StringSlice) MarshalText() ([]byte, error) {
	return []byte(strings.Join(s, ",")), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (s *StringSlice) UnmarshalText(text []byte) error {
	*s = StringSlice(strings.Split(string(text), ","))

	return nil
}

// Value implements the sql driver.Valuer interface.
func (s StringSlice) Value() (driver.Value, error) {
	return strings.Join(s, ","), nil
}

// Scan implements the sql.Scanner interface.
func (s *StringSlice) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid server type")
	}

	switch v := value.(type) {
	case string:
		return s.UnmarshalText([]byte(v))
	case []byte:
		return s.UnmarshalText(v)
	default:
		return fmt.Errorf("type %T is not supported for server type", value)
	}
}
