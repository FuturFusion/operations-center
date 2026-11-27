package image

import (
	"context"
	"net/url"
	"regexp"
	"slices"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

// The source name is used in image paths, file paths and API paths.
var incusImageSourceNameRegexp = regexp.MustCompile(`^[a-z0-9_]+$`)

// "create" clashes with the UI route to create a source.
var reservedIncusImageSourceNames = []string{api.IncusImageSourceLocal, "create"}

// ValidateIncusImageSourceName checks that name only contains lower case
// letters, digits and underscores and is not reserved.
func ValidateIncusImageSourceName(name string) error {
	if name == "" {
		return domain.NewValidationErrf("Invalid source, name can not be empty")
	}

	if !incusImageSourceNameRegexp.MatchString(name) {
		return domain.NewValidationErrf("Invalid source, name %q can only contain lower case letters, digits and underscores", name)
	}

	if slices.Contains(reservedIncusImageSourceNames, name) {
		return domain.NewValidationErrf("Invalid source, name %q is reserved", name)
	}

	return nil
}

type IncusImageSource struct {
	ID               int64  `json:"-"`
	Name             string `json:"name" db:"primary=yes"`
	URL              string `json:"url"`
	FilterExpression string `json:"filter_expression"`

	LastUpdated time.Time `json:"-" expr:"last_updated" db:"update_timestamp"`
}

func (s IncusImageSource) Validate(ctx context.Context, incusImageSource IncusImageSourcePort) error {
	err := ValidateIncusImageSourceName(s.Name)
	if err != nil {
		return err
	}

	// The URL is unique. The local source uses the empty URL.
	err = validateIncusImageSourceURL(s.URL)
	if err != nil {
		return err
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

func validateIncusImageSourceURL(rawURL string) error {
	if rawURL == "" {
		return domain.NewValidationErrf("Invalid source, URL can not be empty")
	}

	sourceURL, err := url.Parse(rawURL)
	if err != nil {
		return domain.NewValidationErrf("Invalid URL: %v", err)
	}

	// The image source is fetched with HTTP.
	if sourceURL.Scheme != "http" && sourceURL.Scheme != "https" {
		return domain.NewValidationErrf("Invalid URL %q, the scheme needs to be http or https", rawURL)
	}

	return nil
}

type IncusImageSourceFilter struct {
	ID   *int
	Name *string
}

type IncusImageSources []IncusImageSource
