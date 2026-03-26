package provisioning_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
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
			t.Cleanup(lifecycle.UpdatesValidateSignal.Reset)

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
			t.Cleanup(lifecycle.UpdatesValidateSignal.Reset)

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
			t.Cleanup(lifecycle.UpdatesValidateSignal.Reset)

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
			t.Cleanup(lifecycle.UpdatesValidateSignal.Reset)

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
			t.Cleanup(lifecycle.UpdatesValidateSignal.Reset)

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
			t.Cleanup(lifecycle.UpdatesValidateSignal.Reset)

			// Run test
			err := channelSvc.DeleteByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestChannelService_GetChangelog(t *testing.T) {
	updateV1UUID := uuidgen.FromPattern(t, "1")
	updateV2UUID := uuidgen.FromPattern(t, "2")
	updateV3UUID := uuidgen.FromPattern(t, "3")

	tests := []struct {
		name                      string
		nameArg                   string
		architectureArg           images.UpdateFileArchitecture
		updateSvcGetAllWithFilter []queue.Item[provisioning.Updates]
		updateSvcGetChangelog     []queue.Item[api.UpdateChangelog]

		assertErr     require.ErrorAssertionFunc
		wantChangelog api.UpdateChangelogs
	}{
		{
			name:            "success",
			nameArg:         "stable",
			architectureArg: images.UpdateFileArchitecture64BitX86,
			updateSvcGetAllWithFilter: []queue.Item[provisioning.Updates]{
				{
					Value: provisioning.Updates{
						{
							UUID:    updateV1UUID,
							Version: "1",
						},
						{
							UUID:    updateV2UUID,
							Version: "2",
						},
						{
							UUID:    updateV3UUID,
							Version: "3",
						},
					},
				},
			},
			updateSvcGetChangelog: []queue.Item[api.UpdateChangelog]{
				{
					Value: api.UpdateChangelog{
						CurrentVersion: "3",
						PriorVersion:   "2",
						Components: map[string]images.ChangelogEntries{
							"foo": {
								Updated: []string{"file version 2 to version 3"},
							},
						},
					},
				},
				{
					Value: api.UpdateChangelog{
						CurrentVersion: "2",
						PriorVersion:   "1",
						Components: map[string]images.ChangelogEntries{
							"foo": {
								Updated: []string{"file version 1 to version 2"},
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			wantChangelog: api.UpdateChangelogs{
				{
					CurrentVersion: "3",
					PriorVersion:   "2",
					Components: map[string]images.ChangelogEntries{
						"foo": {
							Updated: []string{"file version 2 to version 3"},
						},
					},
				},
				{
					CurrentVersion: "2",
					PriorVersion:   "1",
					Components: map[string]images.ChangelogEntries{
						"foo": {
							Updated: []string{"file version 1 to version 2"},
						},
					},
				},
			},
		},
		{
			name:            "success - single entry",
			nameArg:         "stable",
			architectureArg: images.UpdateFileArchitecture64BitX86,
			updateSvcGetAllWithFilter: []queue.Item[provisioning.Updates]{
				{
					Value: provisioning.Updates{
						{
							UUID:    updateV1UUID,
							Version: "1",
						},
					},
				},
			},
			updateSvcGetChangelog: []queue.Item[api.UpdateChangelog]{
				{
					Value: api.UpdateChangelog{
						CurrentVersion: "1",
						Components: map[string]images.ChangelogEntries{
							"foo": {
								Added: []string{"file version 1"},
							},
						},
					},
				},
			},

			assertErr: require.NoError,
			wantChangelog: api.UpdateChangelogs{
				{
					CurrentVersion: "1",
					Components: map[string]images.ChangelogEntries{
						"foo": {
							Added: []string{"file version 1"},
						},
					},
				},
			},
		},

		{
			name:            "error - updateSvc.GetAllWithFilter",
			nameArg:         "stable",
			architectureArg: images.UpdateFileArchitecture64BitX86,
			updateSvcGetAllWithFilter: []queue.Item[provisioning.Updates]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name:            "error - no updates in channel",
			nameArg:         "stable",
			architectureArg: images.UpdateFileArchitecture64BitX86,
			updateSvcGetAllWithFilter: []queue.Item[provisioning.Updates]{
				{
					Value: provisioning.Updates{}, // no updates in channel
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
			},
		},
		{
			name:            "error - single entry - updateSvc.GetChangelog",
			nameArg:         "stable",
			architectureArg: images.UpdateFileArchitecture64BitX86,
			updateSvcGetAllWithFilter: []queue.Item[provisioning.Updates]{
				{
					Value: provisioning.Updates{
						{
							UUID:    updateV1UUID,
							Version: "1",
						},
					},
				},
			},
			updateSvcGetChangelog: []queue.Item[api.UpdateChangelog]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name:            "error - updateSvc.GetChangelog",
			nameArg:         "stable",
			architectureArg: images.UpdateFileArchitecture64BitX86,
			updateSvcGetAllWithFilter: []queue.Item[provisioning.Updates]{
				{
					Value: provisioning.Updates{
						{
							UUID:    updateV1UUID,
							Version: "1",
						},
						{
							UUID:    updateV2UUID,
							Version: "2",
						},
					},
				},
			},
			updateSvcGetChangelog: []queue.Item[api.UpdateChangelog]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			updateSvc := &svcMock.UpdateServiceMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return queue.Pop(t, &tc.updateSvcGetAllWithFilter)
				},
				GetChangelogFunc: func(ctx context.Context, currentID, priorID uuid.UUID, architecture images.UpdateFileArchitecture) (api.UpdateChangelog, error) {
					return queue.Pop(t, &tc.updateSvcGetChangelog)
				},
			}

			channelSvc := provisioning.NewChannelService(nil, updateSvc)
			t.Cleanup(lifecycle.UpdatesValidateSignal.Reset)

			// Run test
			changelog, err := channelSvc.GetChangelogByName(t.Context(), tc.nameArg, tc.architectureArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantChangelog, changelog)

			require.Empty(t, tc.updateSvcGetAllWithFilter)
			require.Empty(t, tc.updateSvcGetChangelog)
		})
	}
}
