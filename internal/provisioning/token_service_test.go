package provisioning_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
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

			tokenSvc := provisioning.NewTokenService(repo, nil, "server01:7443",
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

			tokenSvc := provisioning.NewTokenService(repo, nil, "server01:7443")

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

			tokenSvc := provisioning.NewTokenService(repo, nil, "server01:7443")

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

			tokenSvc := provisioning.NewTokenService(repo, nil, "server01:7443")

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

			tokenSvc := provisioning.NewTokenService(repo, nil, "server01:7443")

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

			tokenSvc := provisioning.NewTokenService(repo, nil, "server01:7443")

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

			assertErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorContains(tt, err, "Token exhausted", i...)
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

			assertErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorContains(tt, err, "Token expired", i...)
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

			tokenSvc := provisioning.NewTokenService(repo, nil, "server01:7443")

			// Run test
			err := tokenSvc.Consume(context.Background(), tc.tokenArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestTokenService_GetPreSeedISO(t *testing.T) {
	// t.Fatal("not implemented")
}
