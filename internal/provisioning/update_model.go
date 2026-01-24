package provisioning

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

//
//generate-expr: Update

type Update struct {
	ID               int                    `json:"-"`
	UUID             uuid.UUID              `json:"-" expr:"uuid" db:"primary=yes"`
	Format           string                 `json:"format" db:"ignore"`
	Origin           string                 `json:"origin"`
	Version          string                 `json:"version"`
	PublishedAt      time.Time              `json:"published_at"`
	Severity         images.UpdateSeverity  `json:"severity"`
	Channels         []string               `json:"channels" db:"ignore"`
	UpstreamChannels UpdateUpstreamChannels `json:"upstream_channels"`
	Changelog        string                 `json:"-" expr:"change_log"`
	Files            UpdateFiles            `json:"files"`
	URL              string                 `json:"url"`
	Status           api.UpdateStatus       `json:"-" expr:"status"`
	LastUpdated      time.Time              `json:"-" expr:"last_updated" db:"update_timestamp"`
}

func (u Update) Validate() error {
	_, ok := images.UpdateSeverities[u.Severity]
	if !ok {
		return domain.NewValidationErrf("Invalid update, validation of severity failed: %q is not a valid update severity", u.Severity)
	}

	var updateStatus api.UpdateStatus
	err := updateStatus.UnmarshalText([]byte(u.Status))
	if u.Status == "" || err != nil {
		return domain.NewValidationErrf("Invalid update, validation of status failed: %v", err)
	}

	return nil
}

func (u Update) Components() []images.UpdateFileComponent {
	componentsSet := make(map[images.UpdateFileComponent]struct{}, len(images.UpdateFileComponents))
	for _, file := range u.Files {
		componentsSet[file.Component] = struct{}{}
	}

	components := make([]images.UpdateFileComponent, 0, len(componentsSet))
	for component := range componentsSet {
		components = append(components, component)
	}

	return components
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
	Filename     string                        `json:"filename"`
	Size         int                           `json:"size"`
	Sha256       string                        `json:"sha256"`
	Component    images.UpdateFileComponent    `json:"component"`
	Type         images.UpdateFileType         `json:"type"`
	Architecture images.UpdateFileArchitecture `json:"architecture"`
}

type UpdateFilter struct {
	ID              *int
	UUID            *uuid.UUID
	Channel         *string `db:"ignore"`
	UpstreamChannel *string `db:"ignore"`
	Origin          *string
	Status          *api.UpdateStatus
}

func (f UpdateFilter) AppendToURLValues(query url.Values) url.Values {
	if f.UpstreamChannel != nil {
		query.Add("channel", *f.UpstreamChannel)
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

type UpdateUpstreamChannels []string

// Value implements the sql driver.Valuer interface.
func (c UpdateUpstreamChannels) Value() (driver.Value, error) {
	return strings.Join(c, ","), nil
}

// Scan implements the sql.Scanner interface.
func (c *UpdateUpstreamChannels) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid value for update upstream channels")
	}

	switch v := value.(type) {
	case string:
		*c = strings.Split(v, ",")
		return nil

	case []byte:
		*c = strings.Split(string(v), ",")
		return nil

	default:
		return fmt.Errorf("type %T is not supported for update upstream channels", value)
	}
}
