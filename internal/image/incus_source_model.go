package image

import (
	"context"
	"net/url"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
)

type IncusImageSource struct {
	ID               int64  `json:"-"`
	Name             string `json:"name" db:"primary=yes"`
	URL              string `json:"url"`
	FilterExpression string `json:"filter_expression"`

	LastUpdated time.Time `json:"-" expr:"last_updated" db:"update_timestamp"`
}

func (s IncusImageSource) Validate(ctx context.Context, incusImageSource IncusImageSourcePort) error {
	if s.Name == "" {
		return domain.NewValidationErrf("Invalid source, name can not be empty")
	}

	_, err := url.Parse(s.URL)
	if err != nil {
		return domain.NewValidationErrf("Invalid URL: %v", err)
	}

	if s.FilterExpression == "" {
		return domain.NewValidationErrf(`Empty filter expression is not permitted`)
	}

	err = incusImageSource.ValidateFilterExpression(ctx, s.FilterExpression)
	if err != nil {
		return domain.NewValidationErrf("Invalid source, failed to validate filter expression: %v", err)
	}

	return nil
}

type IncusImageSourceFilter struct {
	ID   *int
	Name *string
}

type IncusImageSources []IncusImageSource
