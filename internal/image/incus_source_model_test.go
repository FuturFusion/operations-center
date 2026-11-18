package image_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/image/adapter/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

func TestImageSource_Validate(t *testing.T) {
	t.Parallel()

	returnMockImageSourcers := func() image.IncusImageSourcePort {
		return &mock.IncusImageSourcePortMock{
			ValidateFilterExpressionFunc: func(ctx context.Context, filterExpression string) error {
				return nil
			},
		}
	}

	tests := []struct {
		name               string
		source             image.IncusImageSource
		setupImageSourcers func() image.IncusImageSourcePort

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			source: image.IncusImageSource{
				Name:             "name",
				URL:              "https://images.linuxcontainers.org",
				FilterExpression: "true",
			},
			setupImageSourcers: returnMockImageSourcers,

			assertErr: require.NoError,
		},
		{
			name: "error - empty name",
			source: image.IncusImageSource{
				Name:             "", // empty name
				URL:              "https://images.linuxcontainers.org",
				FilterExpression: "true",
			},
			setupImageSourcers: returnMockImageSourcers,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - name with invalid characters",
			source: image.IncusImageSource{
				Name:             "my-source",
				URL:              "https://images.linuxcontainers.org",
				FilterExpression: "true",
			},
			setupImageSourcers: returnMockImageSourcers,

			assertErr: errassert.ValidationErrorContains("Invalid source, name \"my-source\" can only contain lower case letters, digits and underscores"),
		},
		{
			name: "error - reserved name local",
			source: image.IncusImageSource{
				Name:             "local",
				URL:              "https://images.linuxcontainers.org",
				FilterExpression: "true",
			},
			setupImageSourcers: returnMockImageSourcers,

			assertErr: errassert.ValidationErrorContains("Invalid source, name \"local\" is reserved"),
		},
		{
			name: "error - reserved name create",
			source: image.IncusImageSource{
				Name:             "create",
				URL:              "https://images.linuxcontainers.org",
				FilterExpression: "true",
			},
			setupImageSourcers: returnMockImageSourcers,

			assertErr: errassert.ValidationErrorContains("Invalid source, name \"create\" is reserved"),
		},
		{
			name: "error - empty url",
			source: image.IncusImageSource{
				Name:             "name",
				URL:              "", // empty
				FilterExpression: "true",
			},
			setupImageSourcers: returnMockImageSourcers,

			assertErr: errassert.ValidationErrorContains("Invalid source, URL can not be empty"),
		},
		{
			name: "error - invalid url",
			source: image.IncusImageSource{
				Name:             "name",
				URL:              ":|\\", // invalid
				FilterExpression: "true",
			},
			setupImageSourcers: returnMockImageSourcers,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - url without http or https scheme",
			source: image.IncusImageSource{
				Name:             "name",
				URL:              "ftp://images.example.com",
				FilterExpression: "true",
			},
			setupImageSourcers: returnMockImageSourcers,

			assertErr: errassert.ValidationErrorContains(`Invalid URL "ftp://images.example.com", the scheme needs to be http or https`),
		},
		{
			name: "error - image source filter empty",
			source: image.IncusImageSource{
				Name:             "name",
				URL:              "https://images.linuxcontainers.org",
				FilterExpression: "",
			},
			setupImageSourcers: func() image.IncusImageSourcePort {
				return &mock.IncusImageSourcePortMock{
					ValidateFilterExpressionFunc: func(ctx context.Context, filterExpression string) error {
						return nil
					},
				}
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Empty filter expression is not permitted")
			},
		},
		{
			name: "error - image source filter validation",
			source: image.IncusImageSource{
				Name:             "name",
				URL:              "https://images.linuxcontainers.org",
				FilterExpression: "true",
			},
			setupImageSourcers: func() image.IncusImageSourcePort {
				return &mock.IncusImageSourcePortMock{
					ValidateFilterExpressionFunc: func(ctx context.Context, filterExpression string) error {
						return boom.Error
					},
				}
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, boom.Error.Error())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			imageSourcers := tc.setupImageSourcers()

			err := tc.source.Validate(t.Context(), imageSourcers)

			tc.assertErr(t, err)
		})
	}
}
