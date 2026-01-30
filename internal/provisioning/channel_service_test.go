package provisioning_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestChannelService_Create(t *testing.T) {
	tests := []struct {
		name                 string
		channel              provisioning.Channel
		repoCreateChannelErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			channel: provisioning.Channel{
				Name: "A",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
			channel: provisioning.Channel{
				Name: "", // invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - repo.Create",
			channel: provisioning.Channel{
				Name: "A",
			},
			repoCreateChannelErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ChannelRepoMock{
				CreateFunc: func(ctx context.Context, newChannel provisioning.Channel) (int64, error) {
					return -1, tc.repoCreateChannelErr
				},
			}

			channelSvc := provisioning.NewChannelService(repo, nil)

			// Run test
			_, err := channelSvc.Create(t.Context(), tc.channel)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestChannelService_GetAll(t *testing.T) {
	tests := []struct {
		name                  string
		repoGetAllChannels    provisioning.Channels
		repoGetAllChannelsErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllChannels: provisioning.Channels{
				provisioning.Channel{
					Name: "A",
				},
				provisioning.Channel{
					Name: "B",
				},
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:                  "error - repo",
			repoGetAllChannelsErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ChannelRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Channels, error) {
					return tc.repoGetAllChannels, tc.repoGetAllChannelsErr
				},
			}

			channelSvc := provisioning.NewChannelService(repo, nil)

			// Run test
			channels, err := channelSvc.GetAll(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, channels, tc.count)
		})
	}
}

func TestChannelService_GetAllNames(t *testing.T) {
	tests := []struct {
		name                      string
		repoGetAllChannelNames    []string
		repoGetAllChannelNamesErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllChannelNames: []string{
				"A",
				"B",
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:                      "error - repo",
			repoGetAllChannelNamesErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ChannelRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllChannelNames, tc.repoGetAllChannelNamesErr
				},
			}

			channelSvc := provisioning.NewChannelService(repo, nil)

			// Run test
			channelNames, err := channelSvc.GetAllNames(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, channelNames, tc.count)
		})
	}
}

func TestUpdateService_GetChannelByName(t *testing.T) {
	tests := []struct {
		name                    string
		nameArg                 string
		repoGetChannelByName    *provisioning.Channel
		repoGetChannelByNameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                 "success",
			nameArg:              "A",
			repoGetChannelByName: &provisioning.Channel{},

			assertErr: require.NoError,
		},
		{
			name:    "error - empty name",
			nameArg: "",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:                    "error - repo",
			nameArg:                 "A",
			repoGetChannelByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ChannelRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
					return tc.repoGetChannelByName, tc.repoGetChannelByNameErr
				},
			}

			channelSvc := provisioning.NewChannelService(repo, nil)

			// Run test
			channels, err := channelSvc.GetByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetChannelByName, channels)
		})
	}
}

func TestChannelService_Update(t *testing.T) {
	tests := []struct {
		name                 string
		channel              provisioning.Channel
		repoUpdateChannelErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			channel: provisioning.Channel{
				Name: "A",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - invalid name",
			channel: provisioning.Channel{
				Name: "", // invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - repo",
			channel: provisioning.Channel{
				Name: "A",
			},
			repoUpdateChannelErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ChannelRepoMock{
				UpdateFunc: func(ctx context.Context, newChannel provisioning.Channel) error {
					return tc.repoUpdateChannelErr
				},
			}

			channelSvc := provisioning.NewChannelService(repo, nil)

			// Run test
			err := channelSvc.Update(t.Context(), tc.channel)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestChannelService_DeleteByName(t *testing.T) {
	tests := []struct {
		name                                        string
		nameArg                                     string
		updateSvcGetUpdatesByAssignedChannelName    provisioning.Updates
		updateSvcGetUpdatesByAssignedChannelNameErr error
		serverSvcGetAll                             provisioning.Servers
		serverSvcGetAllErr                          error
		repoDeleteChannelByNameErr                  error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "A",

			assertErr: require.NoError,
		},
		{
			name:    "error - empty name",
			nameArg: "",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:    "error - channel in use by update",
			nameArg: "A",
			updateSvcGetUpdatesByAssignedChannelName: provisioning.Updates{
				{
					ID: 1,
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:    "error - updateSvc.GetUpdatesByAssignedChannelNameErr",
			nameArg: "A",
			updateSvcGetUpdatesByAssignedChannelNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - channel in use by server",
			nameArg: "A",
			serverSvcGetAll: provisioning.Servers{
				{
					Name: "server",
					VersionData: api.ServerVersionData{
						UpdateChannel: "A",
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:               "error - serverSvc.GetAll",
			nameArg:            "A",
			serverSvcGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                       "error - repo",
			nameArg:                    "A",
			repoDeleteChannelByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.ChannelRepoMock{
				DeleteByNameFunc: func(ctx context.Context, name string) error {
					return tc.repoDeleteChannelByNameErr
				},
			}

			serverSvc := &svcMock.ServerServiceMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return tc.serverSvcGetAll, tc.serverSvcGetAllErr
				},
			}

			updateSvc := &svcMock.UpdateServiceMock{
				GetUpdatesByAssignedChannelNameFunc: func(ctx context.Context, channelName string) (provisioning.Updates, error) {
					return tc.updateSvcGetUpdatesByAssignedChannelName, tc.updateSvcGetUpdatesByAssignedChannelNameErr
				},
			}

			channelSvc := provisioning.NewChannelService(repo, updateSvc)
			channelSvc.SetServerService(serverSvc)

			// Run test
			err := channelSvc.DeleteByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
