package provisioning_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
	"github.com/FuturFusion/operations-center/internal/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

var (
	uuidA = uuid.MustParse(`b32d0079-c48b-4957-b1cb-bef54125c861`)
	uuidB = uuid.MustParse(`464d229b-3069-4a82-bc59-b215a7c6ed1b`)
)

func TestTokenService_Create(t *testing.T) {
	tests := []struct {
		name            string
		token           provisioning.Token
		randomUUIDValue uuid.UUID
		randomUUIDErr   error
		repoCreateErr   error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			token: provisioning.Token{
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
			},
			randomUUIDValue: uuidA,

			assertErr: require.NoError,
		},
		{
			name: "error - random uuid",
			token: provisioning.Token{
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
			},
			randomUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - validation",
			token: provisioning.Token{
				UsesRemaining: -1, // invalid
				ExpireAt:      time.Now().Add(1 * time.Minute),
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - repo.Create",
			token: provisioning.Token{
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
			},
			randomUUIDValue: uuidA,
			repoCreateErr:   boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				CreateFunc: func(ctx context.Context, in provisioning.Token) (int64, error) {
					return 1, tc.repoCreateErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, nil, nil, nil,
				provisioning.WithRandomUUID(func() (uuid.UUID, error) { return tc.randomUUIDValue, tc.randomUUIDErr }),
			)

			// Run test
			_, err := tokenSvc.Create(context.Background(), tc.token)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestTokenService_GetAll(t *testing.T) {
	tests := []struct {
		name             string
		repoGetAllTokens provisioning.Tokens
		repoGetAllErr    error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllTokens: provisioning.Tokens{
				provisioning.Token{
					UUID:          uuidA,
					UsesRemaining: 1,
					ExpireAt:      time.Now().Add(1 * time.Minute),
					Description:   "A",
				},
				provisioning.Token{
					UUID:          uuidB,
					UsesRemaining: 10,
					ExpireAt:      time.Now().Add(1 * time.Minute),
					Description:   "B",
				},
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:          "error - repo",
			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Tokens, error) {
					return tc.repoGetAllTokens, tc.repoGetAllErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, nil, nil, nil)

			// Run test
			tokens, err := tokenSvc.GetAll(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, tokens, tc.count)
		})
	}
}

func TestTokenService_GetAllUUIDs(t *testing.T) {
	tests := []struct {
		name               string
		repoGetAllUUIDs    []uuid.UUID
		repoGetAllUUIDsErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllUUIDs: []uuid.UUID{
				uuid.MustParse("b32d0079-c48b-4957-b1cb-bef54125c861"),
				uuid.MustParse("464d229b-3069-4a82-bc59-b215a7c6ed1b"),
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:               "error - repo",
			repoGetAllUUIDsErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				GetAllUUIDsFunc: func(ctx context.Context) ([]uuid.UUID, error) {
					return tc.repoGetAllUUIDs, tc.repoGetAllUUIDsErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, nil, nil, nil)

			// Run test
			tokenIDs, err := tokenSvc.GetAllUUIDs(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, tokenIDs, tc.count)
		})
	}
}

func TestTokenService_GetByID(t *testing.T) {
	tests := []struct {
		name               string
		idArg              uuid.UUID
		repoGetByUUIDToken *provisioning.Token
		repoGetByUUIDErr   error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "success",
			idArg: uuidA,
			repoGetByUUIDToken: &provisioning.Token{
				UUID:          uuidA,
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
				Description:   "A",
			},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo",
			idArg:            uuidA,
			repoGetByUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				GetByUUIDFunc: func(ctx context.Context, uuid uuid.UUID) (*provisioning.Token, error) {
					return tc.repoGetByUUIDToken, tc.repoGetByUUIDErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, nil, nil, nil)

			// Run test
			token, err := tokenSvc.GetByUUID(context.Background(), tc.idArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByUUIDToken, token)
		})
	}
}

func TestTokenService_Update(t *testing.T) {
	tests := []struct {
		name          string
		token         provisioning.Token
		repoUpdateErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			token: provisioning.Token{
				UUID:          uuidA,
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
				Description:   "A",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - invalid value for uses remaining",
			token: provisioning.Token{
				UUID:          uuidA,
				UsesRemaining: -1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
				Description:   "A",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - repo",
			token: provisioning.Token{
				UUID:          uuidA,
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
				Description:   "A",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				UpdateFunc: func(ctx context.Context, in provisioning.Token) error {
					return tc.repoUpdateErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, nil, nil, nil)

			// Run test
			err := tokenSvc.Update(context.Background(), tc.token)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestTokenService_DeleteByUUID(t *testing.T) {
	tests := []struct {
		name                string
		idArg               uuid.UUID
		repoDeleteByUUIDErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "success",
			idArg: uuidA,

			assertErr: require.NoError,
		},
		{
			name:                "error - repo",
			idArg:               uuidA,
			repoDeleteByUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				DeleteByUUIDFunc: func(ctx context.Context, id uuid.UUID) error {
					return tc.repoDeleteByUUIDErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, nil, nil, nil)

			// Run test
			err := tokenSvc.DeleteByUUID(context.Background(), tc.idArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestTokenService_Consume(t *testing.T) {
	token := uuid.MustParse(`755d4021-c5c7-47f7-a0f7-4732ffd99dc4`)

	tests := []struct {
		name     string
		tokenArg uuid.UUID

		repoGetByUUIDToken *provisioning.Token
		repoGetByUUIDErr   error
		repoUpdateErr      error

		assertErr       require.ErrorAssertionFunc
		wantUsesRemaing int
	}{
		{
			name:     "success",
			tokenArg: token,

			repoGetByUUIDToken: &provisioning.Token{
				ID:            1,
				UUID:          token,
				UsesRemaining: 10,
				ExpireAt:      time.Now().Add(1 * time.Minute),
			},

			assertErr:       require.NoError,
			wantUsesRemaing: 9,
		},
		{
			name:     "error - GetByUUID",
			tokenArg: token,

			repoGetByUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:     "error - token exhausted",
			tokenArg: token,

			repoGetByUUIDToken: &provisioning.Token{
				ID:            1,
				UUID:          token,
				UsesRemaining: 0, // no uses remaining
				ExpireAt:      time.Now().Add(1 * time.Minute),
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, "Token exhausted")
			},
		},
		{
			name:     "error - token exhausted",
			tokenArg: token,

			repoGetByUUIDToken: &provisioning.Token{
				ID:            1,
				UUID:          token,
				UsesRemaining: 10,
				ExpireAt:      time.Now().Add(-1 * time.Minute), // Token expired
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, "Token expired")
			},
		},
		{
			name:     "success",
			tokenArg: token,

			repoGetByUUIDToken: &provisioning.Token{
				ID:            1,
				UUID:          token,
				UsesRemaining: 10,
				ExpireAt:      time.Now().Add(1 * time.Minute),
			},
			repoUpdateErr: boom.Error,

			assertErr:       boom.ErrorIs,
			wantUsesRemaing: 9,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*provisioning.Token, error) {
					return tc.repoGetByUUIDToken, tc.repoGetByUUIDErr
				},
				UpdateFunc: func(ctx context.Context, token provisioning.Token) error {
					require.Equal(t, tc.tokenArg, token.UUID)
					require.Equal(t, tc.wantUsesRemaing, token.UsesRemaining)
					return tc.repoUpdateErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, nil, nil, nil)

			// Run test
			err := tokenSvc.Consume(context.Background(), tc.tokenArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestTokenService_PreparePreSeededImage(t *testing.T) {
	type image struct {
		imageUUID    uuid.UUID
		tokenID      uuid.UUID
		imageType    api.ImageType
		architecture images.UpdateFileArchitecture
		seedConfig   provisioning.TokenImageSeedConfigs
		createdAt    time.Time
	}

	tests := []struct {
		name             string
		tokenArg         uuid.UUID
		imageTypeArg     api.ImageType
		architectureArg  images.UpdateFileArchitecture
		seedConfigArg    provisioning.TokenImageSeedConfigs
		repoGetByUUIDErr error
		existingImages   []image

		assertErr      require.ErrorAssertionFunc
		wantImageCount int
	}{
		{
			name:            "success",
			tokenArg:        uuidA,
			imageTypeArg:    api.ImageTypeISO,
			architectureArg: images.UpdateFileArchitecture64BitX86,
			seedConfigArg:   provisioning.TokenImageSeedConfigs{},

			assertErr:      require.NoError,
			wantImageCount: 1,
		},
		{
			name:            "success - with expired images",
			tokenArg:        uuidA,
			imageTypeArg:    api.ImageTypeISO,
			architectureArg: images.UpdateFileArchitecture64BitX86,
			seedConfigArg:   provisioning.TokenImageSeedConfigs{},
			existingImages: []image{
				{
					imageUUID:    uuidgen.FromPattern(t, "1"),
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig:   provisioning.TokenImageSeedConfigs{},
					createdAt:    time.Now().Add(-6 * time.Minute), // images expire after 5 minutes.
				},
			},

			assertErr:      require.NoError,
			wantImageCount: 1,
		},

		{
			name:            "error - invalid image type",
			imageTypeArg:    api.ImageType(`invalid`), // invalid
			architectureArg: images.UpdateFileArchitecture64BitX86,
			seedConfigArg:   provisioning.TokenImageSeedConfigs{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:            "error - invalid architecture",
			imageTypeArg:    api.ImageTypeISO,
			architectureArg: images.UpdateFileArchitecture("invalid"), // invalid
			seedConfigArg:   provisioning.TokenImageSeedConfigs{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:             "error - repo.GetByUUID",
			tokenArg:         uuidA,
			imageTypeArg:     api.ImageTypeISO,
			architectureArg:  images.UpdateFileArchitecture64BitX86,
			seedConfigArg:    provisioning.TokenImageSeedConfigs{},
			repoGetByUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*provisioning.Token, error) {
					return nil, tc.repoGetByUUIDErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, nil, nil, nil)
			for _, image := range tc.existingImages {
				tokenSvc.AddImage(image.imageUUID, image.tokenID, image.imageType, image.architecture, image.seedConfig, image.createdAt)
			}

			// Run test
			_, err := tokenSvc.PreparePreSeededImage(context.Background(), tc.tokenArg, tc.imageTypeArg, tc.architectureArg, tc.seedConfigArg)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, tokenSvc.GetImages(), tc.wantImageCount)
		})
	}
}

func TestTokenService_GetPreSeededImage(t *testing.T) {
	type image struct {
		imageUUID    uuid.UUID
		tokenID      uuid.UUID
		imageType    api.ImageType
		architecture images.UpdateFileArchitecture
		seedConfig   provisioning.TokenImageSeedConfigs
		createdAt    time.Time
	}

	imageUUID := uuidgen.FromPattern(t, "1")
	updateUUID := uuidgen.FromPattern(t, "10")

	tmpDir := t.TempDir()
	isoGzFilename := filepath.Join(tmpDir, "some.iso.gz")

	f, err := os.Create(isoGzFilename)
	defer func() { _ = f.Close() }()

	require.NoError(t, err)

	_, err = io.WriteString(f, `Foobar`)
	require.NoError(t, err)

	tests := []struct {
		name                                  string
		tokenIDArg                            uuid.UUID
		imageUUIDArg                          uuid.UUID
		existingImages                        []image
		repoGetByUUIDErr                      error
		updateSvcGetAllWithFilterUpdates      provisioning.Updates
		updateSvcGetAllWithFilterErr          error
		updateSvcGetUpdateAllFilesUpdateFiles provisioning.UpdateFiles
		updateSvcGetUpdateAllFilesErr         error
		updateSvcGetFileByFilenameReadCloser  io.ReadCloser
		updateSvcGetFileByFilenameErr         error
		flasherAdapterGenerateSeededImageErr  error

		assertErr            require.ErrorAssertionFunc
		wantFilename         string
		wantApplicationsSeed map[string]any
		wantIncusSeed        map[string]any
		wantImageCount       int
	}{
		{
			name:         "success - remove expired entry",
			tokenIDArg:   uuidgen.FromPattern(t, "2"),
			imageUUIDArg: imageUUID,
			existingImages: []image{
				{
					imageUUID:    imageUUID,
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig:   provisioning.TokenImageSeedConfigs{},
					createdAt:    time.Now(),
				},
				{
					imageUUID:    uuidgen.FromPattern(t, "3"),
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig:   provisioning.TokenImageSeedConfigs{},
					createdAt:    time.Now().Add(-6 * time.Minute), // images expire after 5 minutes.
				},
			},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename:     isoGzFilename,
					Type:         images.UpdateFileTypeImageISO,
					Architecture: images.UpdateFileArchitecture64BitX86,
				},
			},
			updateSvcGetFileByFilenameReadCloser: func() io.ReadCloser {
				f, err := os.Open(isoGzFilename)
				require.NoError(t, err)

				return f
			}(),

			assertErr:    require.NoError,
			wantFilename: "pre-seed-22222222-2222-2222-2222-222222222222.iso",
			wantApplicationsSeed: map[string]any{
				"version": "1",
				"applications": []any{
					map[string]any{
						"name": "incus",
					},
				},
			},
			wantIncusSeed: map[string]any{
				"apply_defaults": false,
				"version":        "1",
			},
			wantImageCount: 0,
		},
		{
			name:         "success - with seeds",
			tokenIDArg:   uuidgen.FromPattern(t, "2"),
			imageUUIDArg: imageUUID,
			existingImages: []image{
				{
					imageUUID:    imageUUID,
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig: provisioning.TokenImageSeedConfigs{
						Applications: map[string]any{
							"version": "1",
							"applications": []any{
								map[string]any{
									"name": "operations-center",
								},
							},
						},
						Incus: map[string]any{
							"version": "1",
							"certificates": []any{
								map[string]any{
									"name":        "admin",
									"type":        "client",
									"certificate": "foobar",
								},
							},
						},
					},
					createdAt: time.Now(),
				},
				{
					imageUUID:    uuidgen.FromPattern(t, "3"),
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig:   provisioning.TokenImageSeedConfigs{},
					createdAt:    time.Now(), // not expired, should stay untouched.
				},
			},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename:     isoGzFilename,
					Type:         images.UpdateFileTypeImageISO,
					Architecture: images.UpdateFileArchitecture64BitX86,
				},
			},
			updateSvcGetFileByFilenameReadCloser: func() io.ReadCloser {
				f, err := os.Open(isoGzFilename)
				require.NoError(t, err)

				return f
			}(),

			assertErr:    require.NoError,
			wantFilename: "pre-seed-22222222-2222-2222-2222-222222222222.iso",
			wantApplicationsSeed: map[string]any{
				"version": "1",
				"applications": []any{
					map[string]any{
						"name": "operations-center",
					},
				},
			},
			wantIncusSeed: map[string]any{
				"apply_defaults": false,
				"version":        "1",
				"certificates": []any{
					map[string]any{
						"name":        "admin",
						"type":        "client",
						"certificate": "foobar",
					},
				},
			},
			wantImageCount: 1,
		},

		{
			name:         "error - image UUID not found",
			imageUUIDArg: uuidgen.FromPattern(t, "9"), // wrong image UUID
			existingImages: []image{
				{
					imageUUID:    imageUUID,
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig:   provisioning.TokenImageSeedConfigs{},
					createdAt:    time.Now(),
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
			wantImageCount: 1,
		},
		{
			name:         "error - image UUID not found",
			tokenIDArg:   uuidgen.FromPattern(t, "9"), // wrong token UUID
			imageUUIDArg: imageUUID,
			existingImages: []image{
				{
					imageUUID:    imageUUID,
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig:   provisioning.TokenImageSeedConfigs{},
					createdAt:    time.Now(),
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(t, err, domain.ErrConstraintViolation)
			},
			wantImageCount: 1,
		},
		{
			name:         "error - repo.GetByUUID",
			tokenIDArg:   uuidgen.FromPattern(t, "2"),
			imageUUIDArg: imageUUID,
			existingImages: []image{
				{
					imageUUID:    imageUUID,
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig:   provisioning.TokenImageSeedConfigs{},
					createdAt:    time.Now(),
				},
			},
			repoGetByUUIDErr: boom.Error,

			assertErr:      boom.ErrorIs,
			wantImageCount: 1,
		},
		{
			name:         "error - updateSvc.GetAll",
			tokenIDArg:   uuidgen.FromPattern(t, "2"),
			imageUUIDArg: imageUUID,
			existingImages: []image{
				{
					imageUUID:    imageUUID,
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig:   provisioning.TokenImageSeedConfigs{},
					createdAt:    time.Now(),
				},
			},
			updateSvcGetAllWithFilterErr: boom.Error,

			assertErr:      boom.ErrorIs,
			wantImageCount: 1,
		},
		{
			name:         "error - updateSvc.GetAll - no updates",
			tokenIDArg:   uuidgen.FromPattern(t, "2"),
			imageUUIDArg: imageUUID,
			existingImages: []image{
				{
					imageUUID:    imageUUID,
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig:   provisioning.TokenImageSeedConfigs{},
					createdAt:    time.Now(),
				},
			},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to get updates: No updates found")
			},
			wantImageCount: 1,
		},
		{
			name:         "error - updateSvc.GetUpdateAllFiles",
			tokenIDArg:   uuidgen.FromPattern(t, "2"),
			imageUUIDArg: imageUUID,
			existingImages: []image{
				{
					imageUUID:    imageUUID,
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig:   provisioning.TokenImageSeedConfigs{},
					createdAt:    time.Now(),
				},
			},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesErr: boom.Error,

			assertErr:      boom.ErrorIs,
			wantImageCount: 1,
		},
		{
			name:         "error - updateSvc.GetUpdateAllFiles - no files",
			tokenIDArg:   uuidgen.FromPattern(t, "2"),
			imageUUIDArg: imageUUID,
			existingImages: []image{
				{
					imageUUID:    imageUUID,
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig:   provisioning.TokenImageSeedConfigs{},
					createdAt:    time.Now(),
				},
			},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to find image file of type "iso" for architecture "x86_64" in latest update "10101010-1010-1010-1010-101010101010"`)
			},
			wantImageCount: 1,
		},
		{
			name:         "error - updateSvc.GetUpdateByFilename",
			tokenIDArg:   uuidgen.FromPattern(t, "2"),
			imageUUIDArg: imageUUID,
			existingImages: []image{
				{
					imageUUID:    imageUUID,
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig:   provisioning.TokenImageSeedConfigs{},
					createdAt:    time.Now(),
				},
			},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename:     isoGzFilename,
					Type:         images.UpdateFileTypeImageISO,
					Architecture: images.UpdateFileArchitecture64BitX86,
				},
			},
			updateSvcGetFileByFilenameErr: boom.Error,

			assertErr:      boom.ErrorIs,
			wantImageCount: 1,
		},
		{
			name:         "error - updateSvc.GetUpdateByFilename not *os.File",
			tokenIDArg:   uuidgen.FromPattern(t, "2"),
			imageUUIDArg: imageUUID,
			existingImages: []image{
				{
					imageUUID:    imageUUID,
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig:   provisioning.TokenImageSeedConfigs{},
					createdAt:    time.Now(),
				},
			},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename:     isoGzFilename,
					Type:         images.UpdateFileTypeImageISO,
					Architecture: images.UpdateFileArchitecture64BitX86,
				},
			},
			updateSvcGetFileByFilenameReadCloser: func() io.ReadCloser {
				return io.NopCloser(bytes.NewBufferString(``))
			}(),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "is not a file")
			},
			wantImageCount: 1,
		},
		{
			name:         "error - flasher.GenerateSeededImage",
			tokenIDArg:   uuidgen.FromPattern(t, "2"),
			imageUUIDArg: imageUUID,
			existingImages: []image{
				{
					imageUUID:    imageUUID,
					tokenID:      uuidgen.FromPattern(t, "2"),
					imageType:    api.ImageTypeISO,
					architecture: images.UpdateFileArchitecture64BitX86,
					seedConfig:   provisioning.TokenImageSeedConfigs{},
					createdAt:    time.Now(),
				},
			},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename:     isoGzFilename,
					Type:         images.UpdateFileTypeImageISO,
					Architecture: images.UpdateFileArchitecture64BitX86,
				},
			},
			updateSvcGetFileByFilenameReadCloser: func() io.ReadCloser {
				f, err := os.Open(isoGzFilename)
				require.NoError(t, err)

				return f
			}(),
			flasherAdapterGenerateSeededImageErr: boom.Error,

			assertErr: boom.ErrorIs,
			wantApplicationsSeed: map[string]any{
				"version": "1",
				"applications": []any{
					map[string]any{
						"name": "incus",
					},
				},
			},
			wantIncusSeed: map[string]any{
				"apply_defaults": false,
				"version":        "1",
			},
			wantImageCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*provisioning.Token, error) {
					return nil, tc.repoGetByUUIDErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return tc.updateSvcGetAllWithFilterUpdates, tc.updateSvcGetAllWithFilterErr
				},
				GetUpdateAllFilesFunc: func(ctx context.Context, id uuid.UUID) (provisioning.UpdateFiles, error) {
					return tc.updateSvcGetUpdateAllFilesUpdateFiles, tc.updateSvcGetUpdateAllFilesErr
				},
				GetUpdateFileByFilenameFunc: func(ctx context.Context, id uuid.UUID, filename string) (io.ReadCloser, int, error) {
					return tc.updateSvcGetFileByFilenameReadCloser, -1, tc.updateSvcGetFileByFilenameErr
				},
			}

			flasherAdapter := &adapterMock.FlasherPortMock{
				GenerateSeededImageFunc: func(ctx context.Context, id uuid.UUID, seedConfig provisioning.TokenImageSeedConfigs, rc io.ReadCloser) (io.ReadCloser, error) {
					require.Equal(t, tc.wantApplicationsSeed, seedConfig.Applications)
					require.Equal(t, tc.wantIncusSeed, seedConfig.Incus)
					return rc, tc.flasherAdapterGenerateSeededImageErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, updateSvc, nil, flasherAdapter)
			for _, image := range tc.existingImages {
				tokenSvc.AddImage(image.imageUUID, image.tokenID, image.imageType, image.architecture, image.seedConfig, image.createdAt)
			}

			// Run test
			rc, filename, err := tokenSvc.GetPreSeededImage(context.Background(), tc.tokenIDArg, tc.imageUUIDArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantFilename, filename)
			require.Len(t, tokenSvc.GetImages(), tc.wantImageCount)

			if rc != nil {
				defer rc.Close()

				body, err := io.ReadAll(rc)
				require.NoError(t, err)
				require.Equal(t, `Foobar`, string(body))
			}
		})
	}
}

func TestTokenService_GetTokenProviderConfig(t *testing.T) {
	tests := []struct {
		name                        string
		flasherGetProviderConfig    *api.TokenProviderConfig
		flasherGetProviderConfigErr error

		assertErr require.ErrorAssertionFunc
		want      *api.TokenProviderConfig
	}{
		{
			name: "success",
			flasherGetProviderConfig: &api.TokenProviderConfig{
				Version: "1",
			},

			assertErr: require.NoError,
			want: &api.TokenProviderConfig{
				Version: "1",
			},
		},
		{
			name: "error - flasher.GetProviderConfig",
			flasherGetProviderConfig: &api.TokenProviderConfig{
				Version: "1",
			},
			flasherGetProviderConfigErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			flasher := &adapterMock.FlasherPortMock{
				GetProviderConfigFunc: func(ctx context.Context, id uuid.UUID) (*api.TokenProviderConfig, error) {
					return tc.flasherGetProviderConfig, tc.flasherGetProviderConfigErr
				},
			}

			tokenSvc := provisioning.NewTokenService(nil, nil, nil, flasher)

			// Run test
			got, err := tokenSvc.GetTokenProviderConfig(t.Context(), uuidgen.FromPattern(t, "1"))

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTokenService_CreateTokenSeed(t *testing.T) {
	tests := []struct {
		name                   string
		tokenSeed              provisioning.TokenSeed
		repoCreateTokenSeedErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			tokenSeed: provisioning.TokenSeed{
				Name: "config",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - empty name",
			tokenSeed: provisioning.TokenSeed{
				Name: "", // invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr)
			},
		},
		{
			name: "error - repo.Create",
			tokenSeed: provisioning.TokenSeed{
				Name: "config",
			},
			repoCreateTokenSeedErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				CreateTokenSeedFunc: func(ctx context.Context, seedConfig provisioning.TokenSeed) (int64, error) {
					return 1, tc.repoCreateTokenSeedErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, nil, nil, nil)

			// Run test
			_, err := tokenSvc.CreateTokenSeed(t.Context(), tc.tokenSeed)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestTokenService_GetTokenSeedsAll(t *testing.T) {
	tests := []struct {
		name                      string
		repoGetTokenSeedAllTokens provisioning.TokenSeeds
		repoGetTokenSeedAllErr    error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetTokenSeedAllTokens: provisioning.TokenSeeds{
				provisioning.TokenSeed{
					ID:    1,
					Token: uuidgen.FromPattern(t, "1"),
					Name:  "1",
				},
				provisioning.TokenSeed{
					ID:    2,
					Token: uuidgen.FromPattern(t, "1"),
					Name:  "2",
				},
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:                   "error - repo",
			repoGetTokenSeedAllErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				GetTokenSeedAllFunc: func(ctx context.Context, id uuid.UUID) (provisioning.TokenSeeds, error) {
					return tc.repoGetTokenSeedAllTokens, tc.repoGetTokenSeedAllErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, nil, nil, nil)

			// Run test
			tokens, err := tokenSvc.GetTokenSeedAll(context.Background(), uuidgen.FromPattern(t, "1"))

			// Assert
			tc.assertErr(t, err)
			require.Len(t, tokens, tc.count)
		})
	}
}

func TestTokenService_GetTokenSeedAllNames(t *testing.T) {
	tests := []struct {
		name                        string
		idArg                       uuid.UUID
		repoGetTokenSeedAllNames    []string
		repoGetTokenSeedAllNamesErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name:  "success",
			idArg: uuidgen.FromPattern(t, "1"),
			repoGetTokenSeedAllNames: []string{
				"config 1",
				"config 2",
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:                        "error - repo",
			idArg:                       uuidgen.FromPattern(t, "1"),
			repoGetTokenSeedAllNamesErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				GetTokenSeedAllNamesFunc: func(ctx context.Context, id uuid.UUID) ([]string, error) {
					return tc.repoGetTokenSeedAllNames, tc.repoGetTokenSeedAllNamesErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, nil, nil, nil)

			// Run test
			tokenIDs, err := tokenSvc.GetTokenSeedAllNames(context.Background(), tc.idArg)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, tokenIDs, tc.count)
		})
	}
}

func TestTokenService_GetTokenSeedByName(t *testing.T) {
	tests := []struct {
		name                      string
		idArg                     uuid.UUID
		nameArg                   string
		repoGetTokenSeedByName    *provisioning.TokenSeed
		repoGetTokenSeedByNameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			idArg:   uuidgen.FromPattern(t, "1"),
			nameArg: "config",
			repoGetTokenSeedByName: &provisioning.TokenSeed{
				ID:    1,
				Token: uuidgen.FromPattern(t, "1"),
				Name:  "config",
			},

			assertErr: require.NoError,
		},
		{
			name:                      "error",
			idArg:                     uuidgen.FromPattern(t, "1"),
			nameArg:                   "config",
			repoGetTokenSeedByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
					return tc.repoGetTokenSeedByName, tc.repoGetTokenSeedByNameErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, nil, nil, nil)

			// Run test
			tokenSeed, err := tokenSvc.GetTokenSeedByName(t.Context(), tc.idArg, tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetTokenSeedByName, tokenSeed)
		})
	}
}

func TestTokenService_UpdateTokenSeed(t *testing.T) {
	tests := []struct {
		name                   string
		token                  provisioning.TokenSeed
		repoUpdateTokenSeedErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			token: provisioning.TokenSeed{
				ID:    1,
				Token: uuidA,
				Name:  "1",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - empty name",
			token: provisioning.TokenSeed{
				ID:    1,
				Token: uuidA,
				Name:  "", // invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr)
			},
		},
		{
			name: "error - repo",
			token: provisioning.TokenSeed{
				ID:    1,
				Token: uuidA,
				Name:  "1",
			},
			repoUpdateTokenSeedErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				UpdateTokenSeedFunc: func(ctx context.Context, tokenSeedConfig provisioning.TokenSeed) error {
					return tc.repoUpdateTokenSeedErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, nil, nil, nil)

			// Run test
			err := tokenSvc.UpdateTokenSeed(context.Background(), tc.token)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestTokenService_DeleteTokenSeedByUUID(t *testing.T) {
	tests := []struct {
		name                         string
		idArg                        uuid.UUID
		nameArg                      string
		repoDeleteTokenSeedByNameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			idArg:   uuidA,
			nameArg: "1",

			assertErr: require.NoError,
		},
		{
			name:                         "error - repo",
			idArg:                        uuidA,
			nameArg:                      "1",
			repoDeleteTokenSeedByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				DeleteTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) error {
					return tc.repoDeleteTokenSeedByNameErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, nil, nil, nil)

			// Run test
			err := tokenSvc.DeleteTokenSeedByName(context.Background(), tc.idArg, tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestTokenService_GetTokenImageFromTokenSeed(t *testing.T) {
	updateUUID := uuid.MustParse(`00219aa8-ae44-4306-927e-728a2f780836`)

	tmpDir := t.TempDir()
	isoGzFilename := filepath.Join(tmpDir, "some.iso.gz")

	f, err := os.Create(isoGzFilename)
	defer func() { _ = f.Close() }()

	require.NoError(t, err)

	_, err = io.WriteString(f, `Foobar`)
	require.NoError(t, err)

	tests := []struct {
		name                                  string
		imageTypeArg                          api.ImageType
		architectureArg                       images.UpdateFileArchitecture
		repoGetByUUIDErr                      error
		repoGetTokenSeedByName                *provisioning.TokenSeed
		repoGetTokenSeedByNameErr             error
		updateSvcGetAllWithFilterUpdates      provisioning.Updates
		updateSvcGetAllWithFilterErr          error
		updateSvcGetUpdateAllFilesUpdateFiles provisioning.UpdateFiles
		updateSvcGetUpdateAllFilesErr         error
		updateSvcGetFileByFilenameReadCloser  io.ReadCloser
		updateSvcGetFileByFilenameErr         error
		channelSvcGetByNameErr                error
		flasherAdapterGenerateSeededImageErr  error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                   "success",
			imageTypeArg:           api.ImageTypeISO,
			architectureArg:        images.UpdateFileArchitecture64BitX86,
			repoGetTokenSeedByName: &provisioning.TokenSeed{},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename:     isoGzFilename,
					Type:         images.UpdateFileTypeImageISO,
					Architecture: images.UpdateFileArchitecture64BitX86,
				},
			},
			updateSvcGetFileByFilenameReadCloser: func() io.ReadCloser {
				f, err := os.Open(isoGzFilename)
				require.NoError(t, err)

				return f
			}(),

			assertErr: require.NoError,
		},
		{
			name:            "success - with update channel from seed",
			imageTypeArg:    api.ImageTypeISO,
			architectureArg: images.UpdateFileArchitecture64BitX86,
			repoGetTokenSeedByName: &provisioning.TokenSeed{
				Seeds: provisioning.TokenImageSeedConfigs{
					Update: map[string]any{
						"channel": "production",
					},
				},
			},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename:     isoGzFilename,
					Type:         images.UpdateFileTypeImageISO,
					Architecture: images.UpdateFileArchitecture64BitX86,
				},
			},
			updateSvcGetFileByFilenameReadCloser: func() io.ReadCloser {
				f, err := os.Open(isoGzFilename)
				require.NoError(t, err)

				return f
			}(),

			assertErr: require.NoError,
		},

		{
			name:            "error - invalid image type",
			imageTypeArg:    api.ImageType(`invalid`), // invalid
			architectureArg: images.UpdateFileArchitecture64BitX86,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:            "error - invalid architecture",
			imageTypeArg:    api.ImageTypeISO,
			architectureArg: images.UpdateFileArchitecture("invalid"), // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:             "error - repo.GetByUUID",
			imageTypeArg:     api.ImageTypeISO,
			architectureArg:  images.UpdateFileArchitecture64BitX86,
			repoGetByUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                      "error - repo.GetTokenSeedByName",
			imageTypeArg:              api.ImageTypeISO,
			architectureArg:           images.UpdateFileArchitecture64BitX86,
			repoGetTokenSeedByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                         "error - updateSvc.GetAll",
			imageTypeArg:                 api.ImageTypeISO,
			architectureArg:              images.UpdateFileArchitecture64BitX86,
			repoGetTokenSeedByName:       &provisioning.TokenSeed{},
			updateSvcGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                             "error - updateSvc.GetAll - no updates",
			imageTypeArg:                     api.ImageTypeISO,
			architectureArg:                  images.UpdateFileArchitecture64BitX86,
			repoGetTokenSeedByName:           &provisioning.TokenSeed{},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to get updates: No updates found")
			},
		},
		{
			name:                   "error - updateSvc.GetUpdateAllFiles",
			imageTypeArg:           api.ImageTypeISO,
			architectureArg:        images.UpdateFileArchitecture64BitX86,
			repoGetTokenSeedByName: &provisioning.TokenSeed{},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                   "error - updateSvc.GetUpdateAllFiles - no files",
			imageTypeArg:           api.ImageTypeISO,
			architectureArg:        images.UpdateFileArchitecture64BitX86,
			repoGetTokenSeedByName: &provisioning.TokenSeed{},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Failed to find image file of type "iso" for architecture "x86_64" in latest update "00219aa8-ae44-4306-927e-728a2f780836"`)
			},
		},
		{
			name:                   "error - updateSvc.GetUpdateByFilename",
			imageTypeArg:           api.ImageTypeISO,
			architectureArg:        images.UpdateFileArchitecture64BitX86,
			repoGetTokenSeedByName: &provisioning.TokenSeed{},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename:     isoGzFilename,
					Type:         images.UpdateFileTypeImageISO,
					Architecture: images.UpdateFileArchitecture64BitX86,
				},
			},
			updateSvcGetFileByFilenameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                   "error - updateSvc.GetUpdateByFilename not *os.File",
			imageTypeArg:           api.ImageTypeISO,
			architectureArg:        images.UpdateFileArchitecture64BitX86,
			repoGetTokenSeedByName: &provisioning.TokenSeed{},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename:     isoGzFilename,
					Type:         images.UpdateFileTypeImageISO,
					Architecture: images.UpdateFileArchitecture64BitX86,
				},
			},
			updateSvcGetFileByFilenameReadCloser: func() io.ReadCloser {
				return io.NopCloser(bytes.NewBufferString(``))
			}(),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "is not a file")
			},
		},
		{
			name:            "error - update channel invalid",
			imageTypeArg:    api.ImageTypeISO,
			architectureArg: images.UpdateFileArchitecture64BitX86,
			repoGetTokenSeedByName: &provisioning.TokenSeed{
				Seeds: provisioning.TokenImageSeedConfigs{
					Update: map[string]any{
						"channel": 0, // invalid, not a string
					},
				},
			},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename:     isoGzFilename,
					Type:         images.UpdateFileTypeImageISO,
					Architecture: images.UpdateFileArchitecture64BitX86,
				},
			},
			updateSvcGetFileByFilenameReadCloser: func() io.ReadCloser {
				f, err := os.Open(isoGzFilename)
				require.NoError(t, err)

				return f
			}(),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:            "error - update channel not found",
			imageTypeArg:    api.ImageTypeISO,
			architectureArg: images.UpdateFileArchitecture64BitX86,
			repoGetTokenSeedByName: &provisioning.TokenSeed{
				Seeds: provisioning.TokenImageSeedConfigs{
					Update: map[string]any{
						"channel": "not found",
					},
				},
			},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename:     isoGzFilename,
					Type:         images.UpdateFileTypeImageISO,
					Architecture: images.UpdateFileArchitecture64BitX86,
				},
			},
			updateSvcGetFileByFilenameReadCloser: func() io.ReadCloser {
				f, err := os.Open(isoGzFilename)
				require.NoError(t, err)

				return f
			}(),
			channelSvcGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                   "error - flasher.GenerateSeededImage",
			imageTypeArg:           api.ImageTypeISO,
			architectureArg:        images.UpdateFileArchitecture64BitX86,
			repoGetTokenSeedByName: &provisioning.TokenSeed{},
			updateSvcGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename:     isoGzFilename,
					Type:         images.UpdateFileTypeImageISO,
					Architecture: images.UpdateFileArchitecture64BitX86,
				},
			},
			updateSvcGetFileByFilenameReadCloser: func() io.ReadCloser {
				f, err := os.Open(isoGzFilename)
				require.NoError(t, err)

				return f
			}(),
			flasherAdapterGenerateSeededImageErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*provisioning.Token, error) {
					return nil, tc.repoGetByUUIDErr
				},
				GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
					return tc.repoGetTokenSeedByName, tc.repoGetTokenSeedByNameErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return tc.updateSvcGetAllWithFilterUpdates, tc.updateSvcGetAllWithFilterErr
				},
				GetUpdateAllFilesFunc: func(ctx context.Context, id uuid.UUID) (provisioning.UpdateFiles, error) {
					return tc.updateSvcGetUpdateAllFilesUpdateFiles, tc.updateSvcGetUpdateAllFilesErr
				},
				GetUpdateFileByFilenameFunc: func(ctx context.Context, id uuid.UUID, filename string) (io.ReadCloser, int, error) {
					return tc.updateSvcGetFileByFilenameReadCloser, -1, tc.updateSvcGetFileByFilenameErr
				},
			}

			channelSvc := &svcMock.ChannelServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
					return nil, tc.channelSvcGetByNameErr
				},
			}

			flasherAdapter := &adapterMock.FlasherPortMock{
				GenerateSeededImageFunc: func(ctx context.Context, id uuid.UUID, seedConfig provisioning.TokenImageSeedConfigs, rc io.ReadCloser) (io.ReadCloser, error) {
					return rc, tc.flasherAdapterGenerateSeededImageErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, updateSvc, channelSvc, flasherAdapter)

			// Run test
			rc, err := tokenSvc.GetTokenImageFromTokenSeed(context.Background(), uuidgen.FromPattern(t, "1"), "config", tc.imageTypeArg, tc.architectureArg)

			// Assert
			tc.assertErr(t, err)
			if rc != nil {
				defer rc.Close()

				body, err := io.ReadAll(rc)
				require.NoError(t, err)
				require.Equal(t, `Foobar`, string(body))
			}
		})
	}
}
