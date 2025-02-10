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
	uuidA = uuid.Must(uuid.Parse(`b32d0079-c48b-4957-b1cb-bef54125c861`))
	uuidB = uuid.Must(uuid.Parse(`464d229b-3069-4a82-bc59-b215a7c6ed1b`))
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
				CreateFunc: func(ctx context.Context, in provisioning.Token) (provisioning.Token, error) {
					return provisioning.Token{}, tc.repoCreateErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo,
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

			tokenSvc := provisioning.NewTokenService(repo)

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
		name             string
		repoGetAllIDs    []uuid.UUID
		repoGetAllIDsErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllIDs: []uuid.UUID{
				uuid.Must(uuid.Parse("b32d0079-c48b-4957-b1cb-bef54125c861")), uuid.Must(uuid.Parse("464d229b-3069-4a82-bc59-b215a7c6ed1b")),
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:             "error - repo",
			repoGetAllIDsErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				GetAllIDsFunc: func(ctx context.Context) ([]uuid.UUID, error) {
					return tc.repoGetAllIDs, tc.repoGetAllIDsErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo)

			// Run test
			tokenIDs, err := tokenSvc.GetAllIDs(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, tokenIDs, tc.count)
		})
	}
}

func TestTokenService_GetByID(t *testing.T) {
	tests := []struct {
		name             string
		idArg            uuid.UUID
		repoGetByIDToken provisioning.Token
		repoGetByIDErr   error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "success",
			idArg: uuidA,
			repoGetByIDToken: provisioning.Token{
				UUID:          uuidA,
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
				Description:   "A",
			},

			assertErr: require.NoError,
		},
		{
			name:           "error - repo",
			idArg:          uuidA,
			repoGetByIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				GetByIDFunc: func(ctx context.Context, uuid uuid.UUID) (provisioning.Token, error) {
					return tc.repoGetByIDToken, tc.repoGetByIDErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo)

			// Run test
			token, err := tokenSvc.GetByID(context.Background(), tc.idArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByIDToken, token)
		})
	}
}

func TestTokenService_UpdateByID(t *testing.T) {
	tests := []struct {
		name            string
		token           provisioning.Token
		repoUpdateToken provisioning.Token
		repoUpdateErr   error

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
			repoUpdateToken: provisioning.Token{
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
				UpdateByIDFunc: func(ctx context.Context, in provisioning.Token) (provisioning.Token, error) {
					return tc.repoUpdateToken, tc.repoUpdateErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo)

			// Run test
			token, err := tokenSvc.UpdateByID(context.Background(), tc.token)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoUpdateToken, token)
		})
	}
}

func TestTokenService_DeleteByID(t *testing.T) {
	tests := []struct {
		name              string
		idArg             uuid.UUID
		repoDeleteByIDErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "success",
			idArg: uuidA,

			assertErr: require.NoError,
		},
		{
			name:              "error - repo",
			idArg:             uuidA,
			repoDeleteByIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.TokenRepoMock{
				DeleteByIDFunc: func(ctx context.Context, id uuid.UUID) error {
					return tc.repoDeleteByIDErr
				},
			}

			tokenSvc := provisioning.NewTokenService(repo)

			// Run test
			err := tokenSvc.DeleteByID(context.Background(), tc.idArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
