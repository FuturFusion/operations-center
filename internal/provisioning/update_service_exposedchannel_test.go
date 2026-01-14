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

func TestUpdateExposedchannelService_Create(t *testing.T) {
	tests := []struct {
		name                        string
		exposedchannel              provisioning.Exposedchannel
		repoCreateExposedchannelErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			exposedchannel: provisioning.Exposedchannel{
				Name: "A",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
			exposedchannel: provisioning.Exposedchannel{
				Name: "", // invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - repo.Create",
			exposedchannel: provisioning.Exposedchannel{
				Name: "A",
			},
			repoCreateExposedchannelErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				CreateExposedchannelFunc: func(ctx context.Context, newExposedchannel provisioning.Exposedchannel) (int64, error) {
					return -1, tc.repoCreateExposedchannelErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, nil, nil)

			// Run test
			_, err := updateSvc.CreateExposedchannel(t.Context(), tc.exposedchannel)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestUpdateExposedchannelService_GetAll(t *testing.T) {
	tests := []struct {
		name                         string
		repoGetAllExposedchannels    provisioning.Exposedchannels
		repoGetAllExposedchannelsErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllExposedchannels: provisioning.Exposedchannels{
				provisioning.Exposedchannel{
					Name: "A",
				},
				provisioning.Exposedchannel{
					Name: "B",
				},
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:                         "error - repo",
			repoGetAllExposedchannelsErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				GetAllExposedchannelsFunc: func(ctx context.Context) (provisioning.Exposedchannels, error) {
					return tc.repoGetAllExposedchannels, tc.repoGetAllExposedchannelsErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, nil, nil)

			// Run test
			exposedchannels, err := updateSvc.GetAllExposedchannels(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, exposedchannels, tc.count)
		})
	}
}

func TestUpdateExposedchannelService_GetAllNames(t *testing.T) {
	tests := []struct {
		name                             string
		repoGetAllExposedchannelNames    []string
		repoGetAllExposedchannelNamesErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllExposedchannelNames: []string{
				"A",
				"B",
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:                             "error - repo",
			repoGetAllExposedchannelNamesErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				GetAllExposedchannelNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllExposedchannelNames, tc.repoGetAllExposedchannelNamesErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, nil, nil)

			// Run test
			exposedchannelNames, err := updateSvc.GetAllExposedchannelNames(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, exposedchannelNames, tc.count)
		})
	}
}

func TestUpdateService_GetExposedchannelByName(t *testing.T) {
	tests := []struct {
		name                           string
		nameArg                        string
		repoGetExposedchannelByName    *provisioning.Exposedchannel
		repoGetExposedchannelByNameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                        "success",
			nameArg:                     "A",
			repoGetExposedchannelByName: &provisioning.Exposedchannel{},

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
			name:                           "error - repo",
			nameArg:                        "A",
			repoGetExposedchannelByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				GetExposedchannelByNameFunc: func(ctx context.Context, name string) (*provisioning.Exposedchannel, error) {
					return tc.repoGetExposedchannelByName, tc.repoGetExposedchannelByNameErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, nil, nil)

			// Run test
			exposedchannels, err := updateSvc.GetExposedchannelByName(context.Background(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetExposedchannelByName, exposedchannels)
		})
	}
}

func TestUpdateExposedchannelService_Update(t *testing.T) {
	tests := []struct {
		name                        string
		exposedchannel              provisioning.Exposedchannel
		repoUpdateExposedchannelErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			exposedchannel: provisioning.Exposedchannel{
				Name: "A",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - invalid name",
			exposedchannel: provisioning.Exposedchannel{
				Name: "", // invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - repo",
			exposedchannel: provisioning.Exposedchannel{
				Name: "A",
			},
			repoUpdateExposedchannelErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				UpdateExposedchannelFunc: func(ctx context.Context, newExposedchannel provisioning.Exposedchannel) error {
					return tc.repoUpdateExposedchannelErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, nil, nil)

			// Run test
			err := updateSvc.UpdateExposedchannel(t.Context(), tc.exposedchannel)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestUpdateExposedchannelService_Rename(t *testing.T) {
	tests := []struct {
		name                        string
		oldName                     string
		newName                     string
		repoRenameExposedchannelErr error
		serverSvcGetAll             provisioning.Servers
		serverSvcGetAllErr          error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			oldName: "one",
			newName: "one new",

			assertErr: require.NoError,
		},
		{
			name:    "error - old name empty",
			oldName: "", // invalid
			newName: "one new",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:    "error - new name empty",
			oldName: "one",
			newName: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:    "error - old name in use by server",
			oldName: "one",
			newName: "one new",
			serverSvcGetAll: provisioning.Servers{
				{
					Name: "server",
					VersionData: api.ServerVersionData{
						UpdateChannel: "one",
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:               "error - serverSvc.GetAll",
			oldName:            "one",
			newName:            "one new",
			serverSvcGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                        "error - repo.Rename",
			oldName:                     "one",
			newName:                     "one new",
			repoRenameExposedchannelErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				RenameExposedchannelFunc: func(ctx context.Context, oldName, newName string) error {
					require.Equal(t, tc.oldName, oldName)
					require.Equal(t, tc.newName, newName)
					return tc.repoRenameExposedchannelErr
				},
			}

			serverSvc := &svcMock.ServerServiceMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return tc.serverSvcGetAll, tc.serverSvcGetAllErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, nil, nil)
			updateSvc.SetServerService(serverSvc)

			// Run test
			err := updateSvc.RenameExposedchannel(t.Context(), tc.oldName, tc.newName)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestUpdateExposedchannelService_DeleteByName(t *testing.T) {
	tests := []struct {
		name                                          string
		nameArg                                       string
		repoGetUpdatesByAssignedExposedchannelName    provisioning.Updates
		repoGetUpdatesByAssignedExposedchannelNameErr error
		serverSvcGetAll                               provisioning.Servers
		serverSvcGetAllErr                            error
		repoDeleteExposedchannelByNameErr             error

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
			name:    "error - exposed channel in use by update",
			nameArg: "A",
			repoGetUpdatesByAssignedExposedchannelName: provisioning.Updates{
				{
					ID: "update",
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:    "error - serverSvc.GetAll",
			nameArg: "A",
			repoGetUpdatesByAssignedExposedchannelNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - exposed channel in use by server",
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
			name:                              "error - repo",
			nameArg:                           "A",
			repoDeleteExposedchannelByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				GetUpdatesByAssignedExposedchannelNameFunc: func(ctx context.Context, name string) (provisioning.Updates, error) {
					return tc.repoGetUpdatesByAssignedExposedchannelName, tc.repoGetUpdatesByAssignedExposedchannelNameErr
				},
				DeleteExposedchannelByNameFunc: func(ctx context.Context, name string) error {
					return tc.repoDeleteExposedchannelByNameErr
				},
			}

			serverSvc := &svcMock.ServerServiceMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return tc.serverSvcGetAll, tc.serverSvcGetAllErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, nil, nil)
			updateSvc.SetServerService(serverSvc)

			// Run test
			err := updateSvc.DeleteExposedchannelByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
