package image_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
	adapterMock "github.com/FuturFusion/operations-center/internal/image/adapter/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestImageSource_Validate(t *testing.T) {
	returnMockImageSourcers := func() map[api.ImageSourceType]image.ImageSourcerPort {
		return map[api.ImageSourceType]image.ImageSourcerPort{
			api.ImageSourceTypeIncus: &adapterMock.ImageSourcerPortMock{
				ValidateFilterExpressionFunc: func(ctx context.Context, filterExpression string) error {
					return nil
				},
			},
		}
	}

	tests := []struct {
		name               string
		source             image.ImageSource
		setupImageSourcers func() map[api.ImageSourceType]image.ImageSourcerPort

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			source: image.ImageSource{
				Name:             "name",
				URL:              "https://images.linuxcontainers.org",
				Type:             api.ImageSourceTypeIncus,
				FilterExpression: "true",
			},
			setupImageSourcers: returnMockImageSourcers,

			assertErr: require.NoError,
		},
		{
			name: "error - empty name",
			source: image.ImageSource{
				Name:             "", // empty name
				URL:              "https://images.linuxcontainers.org",
				Type:             api.ImageSourceTypeIncus,
				FilterExpression: "true",
			},
			setupImageSourcers: returnMockImageSourcers,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - invalid url",
			source: image.ImageSource{
				Name:             "name",
				URL:              ":|\\", // invalid
				Type:             api.ImageSourceTypeIncus,
				FilterExpression: "true",
			},
			setupImageSourcers: returnMockImageSourcers,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - invalid type",
			source: image.ImageSource{
				Name:             "name",
				URL:              "https://images.linuxcontainers.org",
				Type:             api.ImageSourceType("invalid"), // invalid type
				FilterExpression: "true",
			},
			setupImageSourcers: returnMockImageSourcers,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - no source implementation",
			source: image.ImageSource{
				Name:             "name",
				URL:              "https://images.linuxcontainers.org",
				Type:             api.ImageSourceTypeIncus,
				FilterExpression: "true",
			},
			setupImageSourcers: func() map[api.ImageSourceType]image.ImageSourcerPort {
				return map[api.ImageSourceType]image.ImageSourcerPort{}
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "No source implementation available for source type")
			},
		},
		{
			name: "error - image source filter empty",
			source: image.ImageSource{
				Name:             "name",
				URL:              "https://images.linuxcontainers.org",
				Type:             api.ImageSourceTypeIncus,
				FilterExpression: "",
			},
			setupImageSourcers: func() map[api.ImageSourceType]image.ImageSourcerPort {
				return map[api.ImageSourceType]image.ImageSourcerPort{}
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Empty filter expression is not permitted")
			},
		},
		{
			name: "error - image source filter validation",
			source: image.ImageSource{
				Name:             "name",
				URL:              "https://images.linuxcontainers.org",
				Type:             api.ImageSourceTypeIncus,
				FilterExpression: "true",
			},
			setupImageSourcers: func() map[api.ImageSourceType]image.ImageSourcerPort {
				return map[api.ImageSourceType]image.ImageSourcerPort{
					api.ImageSourceTypeIncus: &adapterMock.ImageSourcerPortMock{
						ValidateFilterExpressionFunc: func(ctx context.Context, filterExpression string) error {
							return boom.Error
						},
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
			imageSourcers := tc.setupImageSourcers()

			err := tc.source.Validate(t.Context(), imageSourcers)

			tc.assertErr(t, err)
		})
	}
}
