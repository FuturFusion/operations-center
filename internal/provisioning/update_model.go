package provisioning

import (
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/shared/api"
)

type Update struct {
	ID          string
	UUID        uuid.UUID `db:"primary=yes"`
	Components  api.UpdateComponents
	Version     string
	PublishedAt time.Time
	Severity    api.UpdateSeverity
	Channel     string
}

type Updates []Update

type UpdateFile struct {
	UpdateID string
	Filename string
	URL      url.URL
	Size     int
}

type UpdateFiles []UpdateFile
