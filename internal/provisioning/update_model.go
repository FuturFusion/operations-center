package provisioning

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/shared/api"
)

type Update struct {
	ID          string             `json:"-"`
	UUID        uuid.UUID          `json:"-" db:"primary=yes"`
	Origin      string             `json:"origin"`
	ExternalID  string             `json:"-"`
	Version     string             `json:"version"`
	PublishedAt time.Time          `json:"published_at"`
	Severity    api.UpdateSeverity `json:"severity"`
	Channel     string             `json:"channel"`
	Changelog   string             `json:"-"`
	Files       UpdateFiles        `json:"files"`
}

type Updates []Update

type UpdateFile struct {
	Filename     string                  `json:"filename"`
	URL          string                  `json:"url"`
	Size         int                     `json:"size"`
	Sha256       string                  `json:"sha256"`
	Component    api.UpdateFileComponent `json:"component"`
	Type         api.UpdateFileType      `json:"type"`
	Architecture api.Architecture        `json:"architecture"`
}

type UpdateFilter struct {
	UUID    *uuid.UUID
	Channel *string
	Origin  *string
}

func (f UpdateFilter) AppendToURLValues(query url.Values) url.Values {
	if f.Channel != nil {
		query.Add("channel", *f.Channel)
	}

	return query
}

func (f UpdateFilter) String() string {
	return f.AppendToURLValues(url.Values{}).Encode()
}

type UpdateFiles []UpdateFile

func (u *UpdateFiles) UnmarshalJSON(data []byte) error {
	return u.UnmarshalText(data)
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (u *UpdateFiles) UnmarshalText(text []byte) error {
	v := []UpdateFile{}
	err := json.Unmarshal(text, &v)
	if err != nil {
		return err
	}

	*u = UpdateFiles(v)
	return nil
}

// Value implements the sql driver.Valuer interface.
func (u UpdateFiles) Value() (driver.Value, error) {
	return json.Marshal(u)
}

// Scan implements the sql.Scanner interface.
func (u *UpdateFiles) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid update file")
	}

	switch v := value.(type) {
	case string:
		return u.UnmarshalText([]byte(v))
	case []byte:
		return u.UnmarshalText(v)
	default:
		return fmt.Errorf("type %T is not supported for update file", value)
	}
}
