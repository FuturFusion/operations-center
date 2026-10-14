package client_test

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_GetTokenImageFromSeed(t *testing.T) {
	const (
		imageData = "pre-seeded-image-data"
		tokenUUID = "b32d0079-c48b-4957-b1cb-bef54125c861"
	)

	tests := []struct {
		name        string
		contentType string
		compress    bool
	}{
		{
			name:        "gzip compressed file",
			contentType: "application/gzip",
			compress:    true,
		},
		{
			name:        "gzip compressed file with media type parameters",
			contentType: "application/gzip; charset=binary",
			compress:    true,
		},
		{
			name:        "uncompressed image",
			contentType: "application/octet-stream",
			compress:    false,
		},
		{
			name:        "uncompressed image without content type",
			contentType: "",
			compress:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotAccept string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.EscapedPath()
				gotAccept = r.Header.Get("Accept")

				if tc.contentType != "" {
					w.Header().Set("Content-Type", tc.contentType)
				}

				if !tc.compress {
					_, _ = w.Write([]byte(imageData))
					return
				}

				gzipWriter := gzip.NewWriter(w)
				defer gzipWriter.Close()

				_, _ = gzipWriter.Write([]byte(imageData))
			}))
			t.Cleanup(server.Close)

			ocClient, err := client.New(server.URL)
			require.NoError(t, err)

			image, err := ocClient.GetTokenImageFromSeed(t.Context(), tokenUUID, "team-seed-1", api.ImageTypeISO, images.UpdateFileArchitecture64BitX86, "stable")
			require.NoError(t, err)

			defer image.Close()

			body, err := io.ReadAll(image)
			require.NoError(t, err)

			require.Equal(t, "application/gzip", gotAccept)
			require.Equal(t, imageData, string(body))
			require.Equal(t, "/1.0/provisioning/tokens/"+tokenUUID+"/seeds/team-seed-1/architecture/x86_64/channel/stable/type/iso/file.iso", gotPath)
		})
	}
}

func Test_GetTokens(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.Token)
	}{
		{
			name:       "success - empty list",
			client:     d.socketClient,
			dbSeedFunc: noop,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Token) {
				t.Helper()

				require.Empty(t, result)
			},
		},
		{
			name:   "success - one record",
			client: d.socketClient,

			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedToken(t, d, uuidgen.FromPattern(t, "1"), "token one")
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.Token) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, uuidgen.FromPattern(t, "1"), result[0].UUID)
				require.Equal(t, "token one", result[0].Description)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result []api.Token) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetTokens(t.Context())

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetToken(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result api.Token)
	}{
		{
			name:   "success - one record",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedToken(t, d, uuidgen.FromPattern(t, "1"), "token one")
			},

			tcNameArg: uuidgen.FromPattern(t, "1").String(),

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result api.Token) {
				t.Helper()

				require.Equal(t, uuidgen.FromPattern(t, "1"), result.UUID)
				require.Equal(t, "token one", result.Description)
				require.Equal(t, defaultChannelName, result.Channel)
				require.Equal(t, 10, result.UsesRemaining)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "1").String(),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result api.Token) {
				t.Helper()
			},
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "2").String(),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: func(t *testing.T, result api.Token) {
				t.Helper()
			},
		},
		{
			name:       "error - invalid uuid",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: "not-a-uuid",

			assertErr: require.Error,
			assertFunc: func(t *testing.T, result api.Token) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetToken(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_CreateToken(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		token api.TokenPut

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:       "success",
			client:     d.socketClient,
			dbSeedFunc: noop,

			token: api.TokenPut{
				UsesRemaining: 5,
				ExpireAt:      time.Now().Add(24 * time.Hour),
				Description:   "new token",
				Channel:       defaultChannelName,
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				tokens, err := d.socketClient.GetTokens(t.Context())
				require.NoError(t, err)
				require.Len(t, tokens, 1)
				require.Equal(t, "new token", tokens[0].Description)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			token: api.TokenPut{
				UsesRemaining: 5,
				ExpireAt:      time.Now().Add(24 * time.Hour),
				Channel:       defaultChannelName,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - validation, expire at in the past",
			client:     d.socketClient,
			dbSeedFunc: noop,

			token: api.TokenPut{
				UsesRemaining: 5,
				ExpireAt:      time.Now().Add(-24 * time.Hour),
				Channel:       defaultChannelName,
			},

			assertErr:  require.Error,
			assertFunc: noop,
		},
		{
			name:       "error - validation, negative uses remaining",
			client:     d.socketClient,
			dbSeedFunc: noop,

			token: api.TokenPut{
				UsesRemaining: -1,
				ExpireAt:      time.Now().Add(24 * time.Hour),
				Channel:       defaultChannelName,
			},

			assertErr:  require.Error,
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.CreateToken(t.Context(), tc.token)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_UpdateToken(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string
		token     api.TokenPut

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedToken(t, d, uuidgen.FromPattern(t, "1"), "token one")
			},

			tcNameArg: uuidgen.FromPattern(t, "1").String(),
			token: api.TokenPut{
				UsesRemaining: 3,
				ExpireAt:      time.Now().Add(48 * time.Hour),
				Description:   "updated token",
				Channel:       defaultChannelName,
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				token, err := d.socketClient.GetToken(t.Context(), uuidgen.FromPattern(t, "1").String())
				require.NoError(t, err)
				require.Equal(t, "updated token", token.Description)
				require.Equal(t, 3, token.UsesRemaining)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "1").String(),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "2").String(),
			token: api.TokenPut{
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(24 * time.Hour),
				Channel:       defaultChannelName,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
		{
			name:       "error - validation",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "1").String(),
			token: api.TokenPut{
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(24 * time.Hour),
				Channel:       "", // invalid, channel can not be empty
			},

			assertErr:  require.Error,
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.UpdateToken(t.Context(), tc.tcNameArg, tc.token)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_DeleteToken(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedToken(t, d, uuidgen.FromPattern(t, "1"), "token one")
			},

			tcNameArg: uuidgen.FromPattern(t, "1").String(),

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetToken(t.Context(), uuidgen.FromPattern(t, "1").String())
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "1").String(),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "2").String(),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.DeleteToken(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func seedToken(t *testing.T, d testDaemon, id uuid.UUID, description string) {
	t.Helper()

	_, err := entities.CreateToken(t.Context(), d.db, provisioning.Token{
		UUID:          id,
		UsesRemaining: 10,
		ExpireAt:      time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second),
		Description:   description,
		Channel:       defaultChannelName,
	})
	require.NoError(t, err)
}

func Test_GetTokenSeeds(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result []api.TokenSeed)
	}{
		{
			name:   "success - empty list",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedToken(t, d, uuidgen.FromPattern(t, "1"), "token one")
			},

			tcNameArg: uuidgen.FromPattern(t, "1").String(),

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.TokenSeed) {
				t.Helper()

				require.Empty(t, result)
			},
		},
		{
			// The token has already been seeded by the previous test case.
			name:   "success - one record",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				_, err := entities.CreateTokenSeed(t.Context(), d.db, provisioning.TokenSeed{
					Token:       uuidgen.FromPattern(t, "1"),
					Name:        "seed-one",
					Description: "seed for seed-one",
					Public:      true,
				})
				require.NoError(t, err)
			},

			tcNameArg: uuidgen.FromPattern(t, "1").String(),

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result []api.TokenSeed) {
				t.Helper()

				require.Len(t, result, 1)
				require.Equal(t, "seed-one", result[0].Name)
				require.Equal(t, uuidgen.FromPattern(t, "1"), result[0].Token)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "1").String(),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result []api.TokenSeed) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetTokenSeeds(t.Context(), tc.tcNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_GetTokenSeed(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg     string
		tcSeedNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T, result api.TokenSeed)
	}{
		{
			name:   "success - one record",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedTokenSeed(t, d, uuidgen.FromPattern(t, "1"), "seed-one")
			},

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcSeedNameArg: "seed-one",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T, result api.TokenSeed) {
				t.Helper()

				require.Equal(t, "seed-one", result.Name)
				require.Equal(t, "seed for seed-one", result.Description)
				require.True(t, result.Public)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcSeedNameArg: "seed-one",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: func(t *testing.T, result api.TokenSeed) {
				t.Helper()
			},
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcSeedNameArg: "unknown",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: func(t *testing.T, result api.TokenSeed) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			result, err := tc.client.GetTokenSeed(t.Context(), tc.tcNameArg, tc.tcSeedNameArg)

			tc.assertErr(t, err)
			tc.assertFunc(t, result)
		})
	}
}

func Test_CreateTokenSeed(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg string
		tokenSeed api.TokenSeedPost

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedToken(t, d, uuidgen.FromPattern(t, "1"), "token one")
			},

			tcNameArg: uuidgen.FromPattern(t, "1").String(),
			tokenSeed: api.TokenSeedPost{
				Name: "new-seed",
				TokenSeedPut: api.TokenSeedPut{
					Description: "new seed",
					Public:      true,
				},
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				tokenSeed, err := d.socketClient.GetTokenSeed(t.Context(), uuidgen.FromPattern(t, "1").String(), "new-seed")
				require.NoError(t, err)
				require.Equal(t, "new seed", tokenSeed.Description)
				require.True(t, tokenSeed.Public)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "1").String(),
			tokenSeed: api.TokenSeedPost{
				Name: "unauthorized",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - validation",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "1").String(),
			tokenSeed: api.TokenSeedPost{
				Name: "", // invalid, no name provided
			},

			assertErr:  require.Error,
			assertFunc: noop,
		},
		{
			name:       "error - conflict",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg: uuidgen.FromPattern(t, "1").String(),
			tokenSeed: api.TokenSeedPost{
				Name: "new-seed", // already exists
			},

			assertErr:  require.Error,
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.CreateTokenSeed(t.Context(), tc.tcNameArg, tc.tokenSeed)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_UpdateTokenSeed(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg     string
		tcSeedNameArg string
		tokenSeed     api.TokenSeedPut

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedTokenSeed(t, d, uuidgen.FromPattern(t, "1"), "seed-one")
			},

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcSeedNameArg: "seed-one",
			tokenSeed: api.TokenSeedPut{
				Description: "updated seed",
				Public:      false,
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				tokenSeed, err := d.socketClient.GetTokenSeed(t.Context(), uuidgen.FromPattern(t, "1").String(), "seed-one")
				require.NoError(t, err)
				require.Equal(t, "updated seed", tokenSeed.Description)
				require.False(t, tokenSeed.Public)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcSeedNameArg: "seed-one",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcSeedNameArg: "unknown",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.UpdateTokenSeed(t.Context(), tc.tcNameArg, tc.tcSeedNameArg, tc.tokenSeed, false)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_RenameTokenSeed(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg     string
		tcSeedNameArg string
		tcNewName     string
		tcForce       bool

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedTokenSeed(t, d, uuidgen.FromPattern(t, "1"), "seed-one")
			},

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcSeedNameArg: "seed-one",
			tcNewName:     "seed-new",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				tokenSeed, err := d.socketClient.GetTokenSeed(t.Context(), uuidgen.FromPattern(t, "1").String(), "seed-new")
				require.NoError(t, err)
				require.Equal(t, "seed-new", tokenSeed.Name)

				_, err = d.socketClient.GetTokenSeed(t.Context(), uuidgen.FromPattern(t, "1").String(), "seed-one")
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
		},
		{
			name:   "success - in use by active deployment with force",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedTokenSeed(t, d, uuidgen.FromPattern(t, "2"), "seed-two")
				seedServerWithActiveDeployment(t, d, "server-two", uuidgen.FromPattern(t, "2"), "seed-two")
			},

			tcNameArg:     uuidgen.FromPattern(t, "2").String(),
			tcSeedNameArg: "seed-two",
			tcNewName:     "seed-new",
			tcForce:       true,

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetTokenSeed(t.Context(), uuidgen.FromPattern(t, "2").String(), "seed-new")
				require.NoError(t, err)
			},
		},
		{
			name:   "error - in use by active deployment",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedTokenSeed(t, d, uuidgen.FromPattern(t, "3"), "seed-one")
				seedServerWithActiveDeployment(t, d, "server-three", uuidgen.FromPattern(t, "3"), "seed-one")
			},

			tcNameArg:     uuidgen.FromPattern(t, "3").String(),
			tcSeedNameArg: "seed-one",
			tcNewName:     "seed-new",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "is in use by the active deployment of servers: server-three")
			},
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetTokenSeed(t.Context(), uuidgen.FromPattern(t, "3").String(), "seed-one")
				require.NoError(t, err)
			},
		},
		{
			name:   "error - invalid new name",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedTokenSeed(t, d, uuidgen.FromPattern(t, "4"), "seed-one")
			},

			tcNameArg:     uuidgen.FromPattern(t, "4").String(),
			tcSeedNameArg: "seed-one",
			tcNewName:     "seed new", // invalid

			assertErr:  require.Error,
			assertFunc: noop,
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcSeedNameArg: "seed-new",
			tcNewName:     "seed-other",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcSeedNameArg: "unknown",
			tcNewName:     "seed-other",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.RenameTokenSeed(t.Context(), tc.tcNameArg, tc.tcSeedNameArg, tc.tcNewName, tc.tcForce)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_DeleteTokenSeed(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name       string
		client     client.OperationsCenterClient
		dbSeedFunc func(t *testing.T)

		tcNameArg     string
		tcSeedNameArg string

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,
			dbSeedFunc: func(t *testing.T) {
				t.Helper()

				seedTokenSeed(t, d, uuidgen.FromPattern(t, "1"), "seed-one")
			},

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcSeedNameArg: "seed-one",

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				_, err := d.socketClient.GetTokenSeed(t.Context(), uuidgen.FromPattern(t, "1").String(), "seed-one")
				require.ErrorIs(t, err, domain.ErrNotFound)
			},
		},
		{
			name:       "error - not authorized",
			client:     d.unauthorizedHTTPClient,
			dbSeedFunc: noop,

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcSeedNameArg: "seed-one",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:       "error - not found",
			client:     d.socketClient,
			dbSeedFunc: noop,

			tcNameArg:     uuidgen.FromPattern(t, "1").String(),
			tcSeedNameArg: "unknown",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound)
			},
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.dbSeedFunc(t)

			err := tc.client.DeleteTokenSeed(t.Context(), tc.tcNameArg, tc.tcSeedNameArg, false)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

// seedServerWithActiveDeployment adds a server with a deployment, which is not
// finished and uses the given token seed.
func seedServerWithActiveDeployment(t *testing.T, d testDaemon, name string, tokenUUID uuid.UUID, seed string) {
	t.Helper()

	_, err := entities.CreateServer(t.Context(), d.db, provisioning.Server{
		Name:         name,
		Channel:      defaultChannelName,
		Status:       api.ServerStatusDeploying,
		StatusDetail: api.ServerStatusDetailNone,
		StatusInternal: provisioning.ServerStatusInternal{
			Deployment: &provisioning.ServerDeployment{
				State: api.ServerDeploymentStateWaitInstall,
				Request: provisioning.ServerDeploymentRequest{
					TokenUUID: tokenUUID,
					Seed:      seed,
					ImageType: api.ImageTypeISO,
				},
			},
		},
	})
	require.NoError(t, err)
}

// seedTokenSeed adds a token with the given UUID together with a token seed
// with the given name.
func seedTokenSeed(t *testing.T, d testDaemon, id uuid.UUID, name string) {
	t.Helper()

	seedToken(t, d, id, "token for "+name)

	_, err := entities.CreateTokenSeed(t.Context(), d.db, provisioning.TokenSeed{
		Token:       id,
		Name:        name,
		Description: "seed for " + name,
		Public:      true,
	})
	require.NoError(t, err)
}
