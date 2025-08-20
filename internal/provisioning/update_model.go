package provisioning

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

type Update struct {
	ID          string             `json:"-"`
	UUID        uuid.UUID          `json:"-" db:"primary=yes"`
	Format      string             `json:"format" db:"ignore"`
	Origin      string             `json:"origin"`
	Version     string             `json:"version"`
	PublishedAt time.Time          `json:"published_at"`
	Severity    api.UpdateSeverity `json:"severity"`
	Channel     string             `json:"channel"`
	Changelog   string             `json:"-"`
	Files       UpdateFiles        `json:"files"`
	URL         string             `json:"url"`
	Status      api.UpdateStatus   `json:"-"`
	LastUpdated time.Time          `json:"-" db:"update_timestamp"`
}

func (u Update) Validate() error {
	var updateSeverity api.UpdateSeverity
	err := updateSeverity.UnmarshalText([]byte(u.Severity))
	if u.Severity == "" || err != nil {
		return domain.NewValidationErrf("Invalid update, validation of severity failed: %v", err)
	}

	var updateStatus api.UpdateStatus
	err = updateStatus.UnmarshalText([]byte(u.Status))
	if u.Status == "" || err != nil {
		return domain.NewValidationErrf("Invalid update, validation of status failed: %v", err)
	}

	return nil
}

type Updates []Update

var _ sort.Interface = Updates{}

func (u Updates) Len() int {
	return len(u)
}

func (u Updates) Less(i, j int) bool {
	iVersion, err := strconv.ParseInt(u[i].Version, 16, 64)
	if err != nil {
		iVersion = math.MinInt // invalid versions are moved to the end.
	}

	jVersion, err := strconv.ParseInt(u[j].Version, 16, 64)
	if err != nil {
		jVersion = math.MinInt // invalid versions are moved to the end.
	}

	// Higher numbers should be returned first and are therefore considered to be less.
	return iVersion > jVersion
}

func (u Updates) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}

type UpdateFile struct {
	Filename     string                  `json:"filename"`
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
	Status  *api.UpdateStatus
}

func (f UpdateFilter) AppendToURLValues(query url.Values) url.Values {
	if f.Channel != nil {
		query.Add("channel", *f.Channel)
	}

	if f.Origin != nil {
		query.Add("origin", *f.Origin)
	}

	if f.Status != nil {
		query.Add("status", f.Status.String())
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

type UsageInformation struct {
	TotalSpaceBytes     uint64
	AvailableSpaceBytes uint64
	UsedSpaceBytes      uint64
}
