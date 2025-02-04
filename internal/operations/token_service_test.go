package operations_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/operations"
	"github.com/FuturFusion/operations-center/internal/operations/repo/mock"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
)

var (
	uuidA = uuid.Must(uuid.Parse(`b32d0079-c48b-4957-b1cb-bef54125c861`))
	uuidB = uuid.Must(uuid.Parse(`464d229b-3069-4a82-bc59-b215a7c6ed1b`))
)

func TestTokenService_Create(t *testing.T) {
	tests := []struct {
		name            string
		token           operations.Token
		randomUUIDValue uuid.UUID
		randomUUIDErr   error
		repoCreateErr   error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			token: operations.Token{
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
			},
			randomUUIDValue: uuidA,

			assertErr: require.NoError,
		},
		{
			name: "error - random uuid",
			token: operations.Token{
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
			},
			randomUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - validation",
			token: operations.Token{
				UsesRemaining: -1, // invalid
				ExpireAt:      time.Now().Add(1 * time.Minute),
			},

			assertErr: func(tt require.TestingT, err error, i ...any) {
				var verr operations.ErrValidation
				require.ErrorAs(tt, err, &verr)
			},
		},
		{
			name: "error - repo.Create",
			token: operations.Token{
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
				CreateFunc: func(ctx context.Context, in operations.Token) (operations.Token, error) {
					return operations.Token{}, tc.repoCreateErr
				},
			}

			tokenSvc := operations.NewTokenService(repo,
				operations.WithRandomUUID(func() (uuid.UUID, error) { return tc.randomUUIDValue, tc.randomUUIDErr }),
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
		repoGetAllTokens operations.Tokens
		repoGetAllErr    error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllTokens: operations.Tokens{
				operations.Token{
					UUID:          uuidA,
					UsesRemaining: 1,
					ExpireAt:      time.Now().Add(1 * time.Minute),
					Description:   "A",
				},
				operations.Token{
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
				GetAllFunc: func(ctx context.Context) (operations.Tokens, error) {
					return tc.repoGetAllTokens, tc.repoGetAllErr
				},
			}

			tokenSvc := operations.NewTokenService(repo)

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
		repoGetAllIDs    []string
		repoGetAllIDsErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllIDs: []string{
				"b32d0079-c48b-4957-b1cb-bef54125c861", "464d229b-3069-4a82-bc59-b215a7c6ed1b",
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
				GetAllIDsFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllIDs, tc.repoGetAllIDsErr
				},
			}

			tokenSvc := operations.NewTokenService(repo)

			// Run test
			inventoryNames, err := tokenSvc.GetAllIDs(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, inventoryNames, tc.count)
		})
	}
}

func TestTokenService_GetByID(t *testing.T) {
	tests := []struct {
		name             string
		idArg            uuid.UUID
		repoGetByIDToken operations.Token
		repoGetByIDErr   error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "success",
			idArg: uuidA,
			repoGetByIDToken: operations.Token{
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
				GetByIDFunc: func(ctx context.Context, uuid uuid.UUID) (operations.Token, error) {
					return tc.repoGetByIDToken, tc.repoGetByIDErr
				},
			}

			tokenSvc := operations.NewTokenService(repo)

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
		token           operations.Token
		repoUpdateToken operations.Token
		repoUpdateErr   error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			token: operations.Token{
				UUID:          uuidA,
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
				Description:   "A",
			},
			repoUpdateToken: operations.Token{
				UUID:          uuidA,
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
				Description:   "A",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - invalid value for uses remaining",
			token: operations.Token{
				UUID:          uuidA,
				UsesRemaining: -1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
				Description:   "A",
			},

			assertErr: func(tt require.TestingT, err error, i ...any) {
				var verr operations.ErrValidation
				require.ErrorAs(tt, err, &verr)
			},
		},
		{
			name: "error - repo",
			token: operations.Token{
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
				UpdateByIDFunc: func(ctx context.Context, in operations.Token) (operations.Token, error) {
					return tc.repoUpdateToken, tc.repoUpdateErr
				},
			}

			tokenSvc := operations.NewTokenService(repo)

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

			tokenSvc := operations.NewTokenService(repo)

			// Run test
			err := tokenSvc.DeleteByID(context.Background(), tc.idArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
