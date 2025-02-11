package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	incusapi "github.com/lxc/incus/v6/shared/api"
)

type StorageVolumeObject incusapi.StorageVolume

// MarshalText implements the encoding.TextMarshaler interface.
func (s StorageVolumeObject) MarshalText() ([]byte, error) {
	return json.Marshal(s)
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (s *StorageVolumeObject) UnmarshalText(text []byte) error {
	var sv incusapi.StorageVolume
	err := json.Unmarshal(text, &sv)
	if err != nil {
		return err
	}

	*s = StorageVolumeObject(sv)

	return nil
}

// Value implements the sql driver.Valuer interface.
func (s StorageVolumeObject) Value() (driver.Value, error) {
	return s.MarshalText()
}

// Scan implements the sql.Scanner interface.
func (s *StorageVolumeObject) Scan(value any) error {
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
