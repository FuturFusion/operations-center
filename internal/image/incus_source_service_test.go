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
)

func TestSourceService_Create(t *testing.T) {
	tests := []struct {
		name                   string
		source                 image.IncusImageSource
		repoCreateTokenSeedErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			source: image.IncusImageSource{
				Name:             "one",
				URL:              "https://images.linuxcontainers.org",
				FilterExpression: "true",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - empty name",
			source: image.IncusImageSource{
				Name:             "", // invalid
				URL:              "https://images.linuxcontainers.org",
				FilterExpression: "true",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr)
			},
		},
		{
			name: "error - repo.Create",
			source: image.IncusImageSource{
				Name:             "one",
				URL:              "https://images.linuxcontainers.org",
				FilterExpression: "true",
			},
			repoCreateTokenSeedErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.IncusImageSourceRepoMock{
				CreateFunc: func(ctx context.Context, source image.IncusImageSource) (int64, error) {
					return 1, tc.repoCreateTokenSeedErr
				},
			}

			incusImageSourcer := &mock.ImageIncusServiceMock{
				ValidateFilterExpressionFunc: func(ctx context.Context, filterExpression string) error {
					return nil
				},
			}

			tokenSvc := image.NewIncusSource(repo, incusImageSourcer)

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
		repoGetAll    image.IncusImageSources
		repoGetAllErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:       "success",
			repoGetAll: image.IncusImageSources{},

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
			repo := &repoMock.IncusImageSourceRepoMock{
				GetAllFunc: func(ctx context.Context) (image.IncusImageSources, error) {
					return tc.repoGetAll, tc.repoGetAllErr
				},
			}

			imageSvc := image.NewIncusSource(repo, nil)

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
			repo := &repoMock.IncusImageSourceRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllNames, tc.repoGetAllNamesErr
				},
			}

			imageSvc := image.NewIncusSource(repo, nil)

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
		repoGetByName    *image.IncusImageSource
		repoGetByNameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:          "success",
			nameArg:       "one",
			repoGetByName: &image.IncusImageSource{},

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
			repo := &repoMock.IncusImageSourceRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*image.IncusImageSource, error) {
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			imageSvc := image.NewIncusSource(repo, nil)

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
		incusImage    image.IncusImageSource
		repoUpdateErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			incusImage: image.IncusImageSource{
				Name:             "one",
				URL:              "https://images.linuxcontainers.org",
				FilterExpression: "true",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
			incusImage: image.IncusImageSource{
				Name: "", // empty name
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr)
			},
		},
		{
			name: "error - repo.Update",
			incusImage: image.IncusImageSource{
				Name:             "one",
				URL:              "https://images.linuxcontainers.org",
				FilterExpression: "true",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.IncusImageSourceRepoMock{
				UpdateFunc: func(ctx context.Context, source image.IncusImageSource) error {
					return tc.repoUpdateErr
				},
			}

			incusImageSourcer := &mock.ImageIncusServiceMock{
				ValidateFilterExpressionFunc: func(ctx context.Context, filterExpression string) error {
					return nil
				},
			}

			imageSvc := image.NewIncusSource(repo, incusImageSourcer)

			// Run test
			err := imageSvc.Update(t.Context(), tc.incusImage)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestSourceService_DeleteByName(t *testing.T) {
	tests := []struct {
		name                          string
		argName                       string
		imageSourcerDeleteBySourceErr error
		repoDeleteByNameErr           error

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
			name:                          "error - imageSourcer.DeleteBySource",
			argName:                       "one",
			imageSourcerDeleteBySourceErr: boom.Error,

			assertErr: boom.ErrorIs,
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
			repo := &repoMock.IncusImageSourceRepoMock{
				DeleteByNameFunc: func(ctx context.Context, name string) error {
					return tc.repoDeleteByNameErr
				},
			}

			incusImageSourcer := &mock.ImageIncusServiceMock{
				DeleteBySourceFunc: func(ctx context.Context, sourceName string) error {
					return tc.imageSourcerDeleteBySourceErr
				},
			}

			imageSvc := image.NewIncusSource(repo, incusImageSourcer)

			// Run test
			err := imageSvc.DeleteByName(t.Context(), tc.argName)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestSourceService_RefreshByName(t *testing.T) {
	tests := []struct {
		name                             string
		repoGetByName                    image.IncusImageSource
		repoGetByNameErr                 error
		imageSourcerRefreshFromSourceErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			repoGetByName: image.IncusImageSource{
				Name: "one",
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByName",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - imageSourcer.RefreshFromSource",
			repoGetByName: image.IncusImageSource{
				Name: "one",
			},
			imageSourcerRefreshFromSourceErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.IncusImageSourceRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*image.IncusImageSource, error) {
					return &tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			incusImageSourcer := &mock.ImageIncusServiceMock{
				RefreshFromSourceFunc: func(ctx context.Context, source image.IncusImageSource) error {
					return tc.imageSourcerRefreshFromSourceErr
				},
			}

			imageSvc := image.NewIncusSource(repo, incusImageSourcer)

			// Run test
			err := imageSvc.RefreshByName(t.Context(), "one")

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestSourceService_RefreshAll(t *testing.T) {
	tests := []struct {
		name                             string
		repoGetAll                       image.IncusImageSources
		repoGetAllErr                    error
		imageSourcerRefreshFromSourceErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			repoGetAll: image.IncusImageSources{
				{
					Name: "one",
				},
			},

			assertErr: require.NoError,
		},
		{
			name:          "error - repo.GetAll",
			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - imageSourcer.RefreshFromSource",
			repoGetAll: image.IncusImageSources{
				{
					Name: "one",
				},
			},
			imageSourcerRefreshFromSourceErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.IncusImageSourceRepoMock{
				GetAllFunc: func(ctx context.Context) (image.IncusImageSources, error) {
					return tc.repoGetAll, tc.repoGetAllErr
				},
			}

			incusImageSourcer := &mock.ImageIncusServiceMock{
				RefreshFromSourceFunc: func(ctx context.Context, source image.IncusImageSource) error {
					return tc.imageSourcerRefreshFromSourceErr
				},
			}

			imageSvc := image.NewIncusSource(repo, incusImageSourcer)

			// Run test
			err := imageSvc.RefreshAll(t.Context())

			// Assert
			tc.assertErr(t, err)
		})
	}
}
