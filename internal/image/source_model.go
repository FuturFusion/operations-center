package image

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

type ImageSource struct {
	ID               int64               `json:"-"`
	Name             string              `json:"name" db:"primary=yes"`
	URL              string              `json:"url"`
	Type             api.ImageSourceType `json:"type" db:"marshal=json"`
	FilterExpression string              `json:"filter_expression"`

	LastUpdated time.Time `json:"-" expr:"last_updated" db:"update_timestamp"`
}

func (s ImageSource) Validate(ctx context.Context, imageSourcers map[api.ImageSourceType]ImageSourcerPort) error {
	if s.Name == "" {
		return domain.NewValidationErrf("Invalid source, name can not be empty")
	}

	_, err := url.Parse(s.URL)
	if err != nil {
		return domain.NewValidationErrf("Invalid URL: %v", err)
	}

	if s.Type != api.ImageSourceTypeIncus {
		return domain.NewValidationErrf(`Invalid source, only type "incus" is supported`)
	}

	if s.FilterExpression == "" {
		return domain.NewValidationErrf(`Empty filter expression is not permitted`)
	}

	imageSourcer, ok := imageSourcers[s.Type]
	if !ok {
		return fmt.Errorf("No source implementation available for source type %q", s.Type)
	}

	err = imageSourcer.ValidateFilterExpression(ctx, s.FilterExpression)
	if err != nil {
		return domain.NewValidationErrf("Invalid source, failed to validate filter expression: %v", err)
	}

	return nil
}

type ImageSourceFilter struct {
	ID   *int
	Name *string
}

type Sources []ImageSource
