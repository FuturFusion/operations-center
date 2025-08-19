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
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
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

			tokenSvc := provisioning.NewTokenService(repo, nil, nil,
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

			tokenSvc := provisioning.NewTokenService(repo, nil, nil)

			// Run test
			tokens, err := tokenSvc.GetAll(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, tokens, tc.count)
		})
	}
}

func TestTokenService_GetAllNames(t *testing.T) {
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

			tokenSvc := provisioning.NewTokenService(repo, nil, nil)

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

			tokenSvc := provisioning.NewTokenService(repo, nil, nil)

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

			tokenSvc := provisioning.NewTokenService(repo, nil, nil)

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

			tokenSvc := provisioning.NewTokenService(repo, nil, nil)

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

			tokenSvc := provisioning.NewTokenService(repo, nil, nil)

			// Run test
			err := tokenSvc.Consume(context.Background(), tc.tokenArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestTokenService_GetPreSeedImage(t *testing.T) {
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
		tokenArg                              uuid.UUID
		seedConfigArg                         provisioning.TokenSeedConfig
		repoGetByUUIDErr                      error
		updateSvcGetAllUpdates                provisioning.Updates
		updateSvcGetAllErr                    error
		updateSvcGetUpdateAllFilesUpdateFiles provisioning.UpdateFiles
		updateSvcGetUpdateAllFilesErr         error
		updateSvcGetFileByFilenameReadCloser  io.ReadCloser
		updateSvcGetFileByFilenameErr         error
		flasherAdapterGenerateSeededImageErr  error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:     "success",
			tokenArg: uuidA,
			seedConfigArg: provisioning.TokenSeedConfig{
				ImageType: api.ImageTypeISO,
			},
			updateSvcGetAllUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename: isoGzFilename,
					Type:     api.UpdateFileTypeImageISO,
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
			name: "error - seedConfig.Validate",
			seedConfigArg: provisioning.TokenSeedConfig{
				ImageType: api.ImageType(`invalid`), // invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:     "error - repo.GetByUUID",
			tokenArg: uuidA,
			seedConfigArg: provisioning.TokenSeedConfig{
				ImageType: api.ImageTypeISO,
			},
			repoGetByUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:     "error - updateSvc.GetAll",
			tokenArg: uuidA,
			seedConfigArg: provisioning.TokenSeedConfig{
				ImageType: api.ImageTypeISO,
			},
			updateSvcGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:     "error - updateSvc.GetAll - no updates",
			tokenArg: uuidA,
			seedConfigArg: provisioning.TokenSeedConfig{
				ImageType: api.ImageTypeISO,
			},
			updateSvcGetAllUpdates: provisioning.Updates{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to get updates: No updates found")
			},
		},
		{
			name:     "error - updateSvc.GetUpdateAllFiles",
			tokenArg: uuidA,
			seedConfigArg: provisioning.TokenSeedConfig{
				ImageType: api.ImageTypeISO,
			},
			updateSvcGetAllUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:     "error - updateSvc.GetUpdateAllFiles - no files",
			tokenArg: uuidA,
			seedConfigArg: provisioning.TokenSeedConfig{
				ImageType: api.ImageTypeISO,
			},
			updateSvcGetAllUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to find image file for latest update")
			},
		},
		{
			name:     "error - updateSvc.GetUpdateByFilename",
			tokenArg: uuidA,
			seedConfigArg: provisioning.TokenSeedConfig{
				ImageType: api.ImageTypeISO,
			},
			updateSvcGetAllUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename: isoGzFilename,
					Type:     api.UpdateFileTypeImageISO,
				},
			},
			updateSvcGetFileByFilenameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:     "error - updateSvc.GetUpdateByFilename not *os.File",
			tokenArg: uuidA,
			seedConfigArg: provisioning.TokenSeedConfig{
				ImageType: api.ImageTypeISO,
			},
			updateSvcGetAllUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename: isoGzFilename,
					Type:     api.UpdateFileTypeImageISO,
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
			name:     "error - flasher.GenerateSeededImage",
			tokenArg: uuidA,
			seedConfigArg: provisioning.TokenSeedConfig{
				ImageType: api.ImageTypeISO,
			},
			updateSvcGetAllUpdates: provisioning.Updates{
				{
					UUID: updateUUID,
				},
			},
			updateSvcGetUpdateAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename: isoGzFilename,
					Type:     api.UpdateFileTypeImageISO,
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
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Updates, error) {
					return tc.updateSvcGetAllUpdates, tc.updateSvcGetAllErr
				},
				GetUpdateAllFilesFunc: func(ctx context.Context, id uuid.UUID) (provisioning.UpdateFiles, error) {
					return tc.updateSvcGetUpdateAllFilesUpdateFiles, tc.updateSvcGetUpdateAllFilesErr
				},
				GetUpdateFileByFilenameFunc: func(ctx context.Context, id uuid.UUID, filename string) (io.ReadCloser, int, error) {
					return tc.updateSvcGetFileByFilenameReadCloser, -1, tc.updateSvcGetFileByFilenameErr
				},
			}

			flasherAdapter := &adapterMock.FlasherPortMock{
				GenerateSeededImageFunc: func(ctx context.Context, id uuid.UUID, seedConfig provisioning.TokenSeedConfig, rc io.ReadCloser) (io.ReadCloser, error) {
					return rc, tc.flasherAdapterGenerateSeededImageErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo, updateSvc, flasherAdapter)

			// Run test
			rc, err := tokenSvc.GetPreSeedImage(context.Background(), tc.tokenArg, tc.seedConfigArg)

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
