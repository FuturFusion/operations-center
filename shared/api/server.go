package api

import (
	"database/sql/driver"
	"fmt"
)

type ServerType string

const (
	ServerTypeUnknown          ServerType = "unknown"
	ServerTypeIncus            ServerType = "incus"
	ServerTypeMigrationManager ServerType = "migration-manager"
	ServerTypeOperationsCenter ServerType = "operations-center"
)

var serverTypes = map[ServerType]struct{}{
	ServerTypeUnknown:          {},
	ServerTypeIncus:            {},
	ServerTypeMigrationManager: {},
	ServerTypeOperationsCenter: {},
}

// MarshalText implements the encoding.TextMarshaler interface.
func (s ServerType) MarshalText() ([]byte, error) {
	return []byte(s), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (s *ServerType) UnmarshalText(text []byte) error {
	_, ok := serverTypes[ServerType(text)]
	if !ok {
		return fmt.Errorf("%q is not a valid server type", string(text))
	}

	*s = ServerType(text)

	return nil
}

// Value implements the sql driver.Valuer interface.
func (s ServerType) Value() (driver.Value, error) {
	return string(s), nil
}

// Scan implements the sql.Scanner interface.
func (s *ServerType) Scan(value any) error {
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
