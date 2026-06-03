package image_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/image/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/image/repo/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestSourceService_Create(t *testing.T) {
	tests := []struct {
		name                   string
		source                 image.ImageSource
		repoCreateTokenSeedErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			source: image.ImageSource{
				Name:             "one",
				URL:              "https://images.linuxcontainers.org",
				Type:             api.ImageSourceTypeIncus,
				FilterExpression: "true",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - empty name",
			source: image.ImageSource{
				Name:             "", // invalid
				URL:              "https://images.linuxcontainers.org",
				Type:             api.ImageSourceTypeIncus,
				FilterExpression: "true",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr)
			},
		},
		{
			name: "error - repo.Create",
			source: image.ImageSource{
				Name:             "one",
				URL:              "https://images.linuxcontainers.org",
				Type:             api.ImageSourceTypeIncus,
				FilterExpression: "true",
			},
			repoCreateTokenSeedErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.SourceRepoMock{
				CreateFunc: func(ctx context.Context, source image.ImageSource) (int64, error) {
					return 1, tc.repoCreateTokenSeedErr
				},
			}

			imageSourcers := map[api.ImageSourceType]image.ImageSourcerPort{
				api.ImageSourceTypeIncus: &mock.ImageIncusServiceMock{
					ValidateFilterExpressionFunc: func(ctx context.Context, filterExpression string) error {
						return nil
					},
				},
			}

			tokenSvc := image.NewSource(repo, imageSourcers)

			// Run test
			_, err := tokenSvc.Create(t.Context(), tc.source)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestSourceService_GetAll(t *testing.T) {
	tests := []struct {
		name          string
		repoGetAll    image.Sources
		repoGetAllErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:       "success",
			repoGetAll: image.Sources{},

			assertErr: require.NoError,
		},
		{
			name:          "error - repo",
			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.SourceRepoMock{
				GetAllFunc: func(ctx context.Context) (image.Sources, error) {
					return tc.repoGetAll, tc.repoGetAllErr
				},
			}

			imageSvc := image.NewSource(repo, nil)

			// Run test
			images, err := imageSvc.GetAll(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetAll, images)
		})
	}
}

func TestSourceService_GetAllNames(t *testing.T) {
	tests := []struct {
		name               string
		repoGetAllNames    []string
		repoGetAllNamesErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			repoGetAllNames: []string{
				"one",
				"two",
			},

			assertErr: require.NoError,
		},
		{
			name:               "error - repo",
			repoGetAllNamesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.SourceRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllNames, tc.repoGetAllNamesErr
				},
			}

			imageSvc := image.NewSource(repo, nil)

			// Run test
			images, err := imageSvc.GetAllNames(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetAllNames, images)
		})
	}
}

func TestSourceService_GetByName(t *testing.T) {
	tests := []struct {
		name             string
		nameArg          string
		repoGetByName    *image.ImageSource
		repoGetByNameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:          "success",
			nameArg:       "one",
			repoGetByName: &image.ImageSource{},

			assertErr: require.NoError,
		},
		{
			name:    "error - empty name",
			nameArg: "", // empty

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Image source name cannot be empty`)
			},
		},
		{
			name:             "error - repo",
			nameArg:          "one",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.SourceRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*image.ImageSource, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			imageSvc := image.NewSource(repo, nil)

			// Run test
			img, err := imageSvc.GetByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByName, img)
		})
	}
}

func TestSourceService_Update(t *testing.T) {
	tests := []struct {
		name          string
		incusImage    image.ImageSource
		repoUpdateErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			incusImage: image.ImageSource{
				Name:             "one",
				URL:              "https://images.linuxcontainers.org",
				Type:             api.ImageSourceTypeIncus,
				FilterExpression: "true",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
			incusImage: image.ImageSource{
				Name: "", // empty name
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr)
			},
		},
		{
			name: "error - repo.Update",
			incusImage: image.ImageSource{
				Name:             "one",
				URL:              "https://images.linuxcontainers.org",
				Type:             api.ImageSourceTypeIncus,
				FilterExpression: "true",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.SourceRepoMock{
				UpdateFunc: func(ctx context.Context, source image.ImageSource) error {
					return tc.repoUpdateErr
				},
			}

			imageSourcers := map[api.ImageSourceType]image.ImageSourcerPort{
				api.ImageSourceTypeIncus: &mock.ImageIncusServiceMock{
					ValidateFilterExpressionFunc: func(ctx context.Context, filterExpression string) error {
						return nil
					},
				},
			}

			imageSvc := image.NewSource(repo, imageSourcers)

			// Run test
			err := imageSvc.Update(t.Context(), tc.incusImage)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestSourceService_DeleteByName(t *testing.T) {
	tests := []struct {
		name                string
		argName             string
		filesRepoDeleteErr  error
		repoDeleteByNameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			argName: "one",

			assertErr: require.NoError,
		},
		{
			name:    "error - invalid name",
			argName: "", // empty name

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Image source name cannot be empty`)
			},
		},
		{
			name:                "error - repo.DeleteByName",
			argName:             "one",
			repoDeleteByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.SourceRepoMock{
				DeleteByNameFunc: func(ctx context.Context, name string) error {
					return tc.repoDeleteByNameErr
				},
			}

			imageSvc := image.NewSource(repo, nil)

			// Run test
			err := imageSvc.DeleteByName(t.Context(), tc.argName)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
