package provisioning_test

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
	"github.com/FuturFusion/operations-center/internal/testing/queue"
	"github.com/FuturFusion/operations-center/internal/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestUpdateService_CreateFromArchive(t *testing.T) {
	tests := []struct {
		name string

		repoUpdateFilesCreateFromArchiveErr    error
		repoUpdateFilesCreateFromArchiveUpdate *provisioning.Update
		repoUpsertErr                          error

		assertErr require.ErrorAssertionFunc
		wantID    uuid.UUID
	}{
		{
			name: "success",

			repoUpdateFilesCreateFromArchiveUpdate: &provisioning.Update{
				UUID:     uuid.MustParse(`98e0ec84-eb21-4406-a7bf-727610d4d0c4`),
				Severity: api.UpdateSeverityLow,
			},

			assertErr: require.NoError,
			wantID:    uuid.MustParse(`98e0ec84-eb21-4406-a7bf-727610d4d0c4`),
		},
		{
			name: "error - CreateFromArchive",

			repoUpdateFilesCreateFromArchiveErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - Validate",

			repoUpdateFilesCreateFromArchiveUpdate: &provisioning.Update{
				UUID: uuid.MustParse(`98e0ec84-eb21-4406-a7bf-727610d4d0c4`),
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - Upsert",

			repoUpdateFilesCreateFromArchiveUpdate: &provisioning.Update{
				UUID:     uuid.MustParse(`98e0ec84-eb21-4406-a7bf-727610d4d0c4`),
				Severity: api.UpdateSeverityLow,
			},
			repoUpsertErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				UpsertFunc: func(ctx context.Context, update provisioning.Update) error {
					return tc.repoUpsertErr
				},
			}

			repoUpdateFiles := &repoMock.UpdateFilesRepoMock{
				CreateFromArchiveFunc: func(ctx context.Context, tarReader *tar.Reader) (*provisioning.Update, error) {
					return tc.repoUpdateFilesCreateFromArchiveUpdate, tc.repoUpdateFilesCreateFromArchiveErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, repoUpdateFiles, nil)

			// Run test
			id, err := updateSvc.CreateFromArchive(context.Background(), nil)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantID, id)
		})
	}
}

func TestUpdateService_CleanupAll(t *testing.T) {
	tests := []struct {
		name                   string
		filesRepoCleanupAllErr error
		repoGetAll             provisioning.Updates
		repoGetAllErr          error
		repoDeleteByUUID       []queue.Item[struct{}]

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			repoGetAll: provisioning.Updates{
				{
					UUID: uuid.MustParse("3b9d0f85-67b4-480e-b369-fef25e9d8ccc"),
				},
				{
					UUID: uuid.MustParse("ce9b4489-cc2e-4726-9103-ea22d07a2110"),
				},
			},
			repoDeleteByUUID: []queue.Item[struct{}]{
				{},
				{},
			},

			assertErr: require.NoError,
		},
		{
			name:                   "error - filesRepo.CleanupAll",
			filesRepoCleanupAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:          "error - repo.GetAll",
			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - repo.DeleteByID",
			repoGetAll: provisioning.Updates{
				{
					UUID: uuid.MustParse("3b9d0f85-67b4-480e-b369-fef25e9d8ccc"),
				},
				{
					UUID: uuid.MustParse("ce9b4489-cc2e-4726-9103-ea22d07a2110"),
				},
			},
			repoDeleteByUUID: []queue.Item[struct{}]{
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
			repo := &repoMock.UpdateRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Updates, error) {
					return tc.repoGetAll, tc.repoGetAllErr
				},
				DeleteByUUIDFunc: func(ctx context.Context, id uuid.UUID) error {
					_, err := queue.Pop(t, &tc.repoDeleteByUUID)
					return err
				},
			}

			repoUpdateFiles := &repoMock.UpdateFilesRepoMock{
				CleanupAllFunc: func(ctx context.Context) error {
					return tc.filesRepoCleanupAllErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, repoUpdateFiles, nil)

			// Run test
			err := updateSvc.CleanupAll(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Empty(t, tc.repoDeleteByUUID)
		})
	}
}

func TestUpdateService_GetAll(t *testing.T) {
	tests := []struct {
		name              string
		repoGetAllUpdates provisioning.Updates
		repoGetAllErr     error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:              "success",
			repoGetAllUpdates: provisioning.Updates{},

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
			repo := &repoMock.UpdateRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Updates, error) {
					return tc.repoGetAllUpdates, tc.repoGetAllErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, nil, nil)

			// Run test
			updates, err := updateSvc.GetAll(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetAllUpdates, updates)
		})
	}
}

func TestUpdateService_GetAllWithFilter(t *testing.T) {
	tests := []struct {
		name                    string
		filter                  provisioning.UpdateFilter
		repoGetAllWithFilter    provisioning.Updates
		repoGetAllWithFilterErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			filter: provisioning.UpdateFilter{
				Origin: ptr.To("one"),
			},
			repoGetAllWithFilter: provisioning.Updates{
				provisioning.Update{
					UUID: uuid.MustParse(`1b6b5509-a9a6-419f-855f-7a8618ce76ad`),
				},
				provisioning.Update{
					UUID: uuid.MustParse(`689396f9-cf05-4776-a567-38014d37f861`),
				},
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name: "success - with channel",
			filter: provisioning.UpdateFilter{
				Origin:  ptr.To("one"),
				Channel: ptr.To("stable"),
			},
			repoGetAllWithFilter: provisioning.Updates{
				provisioning.Update{
					UUID:     uuid.MustParse(`1b6b5509-a9a6-419f-855f-7a8618ce76ad`),
					Channels: []string{"stable", "daily"},
				},
				provisioning.Update{
					UUID:     uuid.MustParse(`689396f9-cf05-4776-a567-38014d37f861`),
					Channels: []string{"daily"},
				},
			},

			assertErr: require.NoError,
			count:     1,
		},
		{
			name:                    "error - repo",
			repoGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Updates, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return tc.repoGetAllWithFilter, tc.repoGetAllWithFilterErr
				},
			}

			serverSvc := provisioning.NewUpdateService(repo, nil, nil)

			// Run test
			server, err := serverSvc.GetAllWithFilter(context.Background(), tc.filter)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, server, tc.count)
		})
	}
}

func TestUpdateService_GetAllUUIDs(t *testing.T) {
	tests := []struct {
		name               string
		repoGetAllUUIDs    []uuid.UUID
		repoGetAllUUIDsErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			repoGetAllUUIDs: []uuid.UUID{
				uuid.MustParse(`8926daa1-3a48-4739-9a82-e32ebd22d343`),
				uuid.MustParse(`84156d67-0bcb-4b60-ac23-2c67f552fb8c`),
			},

			assertErr: require.NoError,
		},
		{
			name:               "error - repo",
			repoGetAllUUIDsErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				GetAllUUIDsFunc: func(ctx context.Context) ([]uuid.UUID, error) {
					return tc.repoGetAllUUIDs, tc.repoGetAllUUIDsErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, nil, nil)

			// Run test
			updates, err := updateSvc.GetAllUUIDs(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetAllUUIDs, updates)
		})
	}
}

func TestUpdateService_GetAllUUIDsWithFilter(t *testing.T) {
	tests := []struct {
		name               string
		filter             provisioning.UpdateFilter
		repoGetAllUUIDs    []uuid.UUID
		repoGetAllUUIDsErr error
		repoGetAll         provisioning.Updates
		repoGetAllErr      error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name:   "success",
			filter: provisioning.UpdateFilter{},
			repoGetAllUUIDs: []uuid.UUID{
				uuid.MustParse(`8926daa1-3a48-4739-9a82-e32ebd22d343`),
				uuid.MustParse(`84156d67-0bcb-4b60-ac23-2c67f552fb8c`),
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name: "success - with channel",
			filter: provisioning.UpdateFilter{
				Channel: ptr.To("stable"),
			},
			repoGetAll: provisioning.Updates{
				{
					UUID:     uuid.MustParse(`8926daa1-3a48-4739-9a82-e32ebd22d343`),
					Channels: []string{"stable", "daily"},
				},
				{
					UUID:     uuid.MustParse(`84156d67-0bcb-4b60-ac23-2c67f552fb8c`),
					Channels: []string{"daily"},
				},
			},

			assertErr: require.NoError,
			count:     1,
		},
		{
			name:               "error - repo",
			repoGetAllUUIDsErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
		{
			name: "error - repo",
			filter: provisioning.UpdateFilter{
				Channel: ptr.To("stable"),
			},
			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Updates, error) {
					return tc.repoGetAll, tc.repoGetAllErr
				},
				GetAllUUIDsFunc: func(ctx context.Context) ([]uuid.UUID, error) {
					return tc.repoGetAllUUIDs, tc.repoGetAllUUIDsErr
				},
			}

			serverSvc := provisioning.NewUpdateService(repo, nil, nil)

			// Run test
			serverIDs, err := serverSvc.GetAllUUIDsWithFilter(context.Background(), tc.filter)

			// Assert
			tc.assertErr(t, err)
			require.Len(t, serverIDs, tc.count)
		})
	}
}

func TestUpdateService_GetByUUID(t *testing.T) {
	tests := []struct {
		name                string
		idArg               uuid.UUID
		repoGetByUUIDUpdate *provisioning.Update
		repoGetByUUIDErr    error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                "success",
			idArg:               uuid.MustParse(`13595731-843c-441e-9cf3-6c2869624cc8`),
			repoGetByUUIDUpdate: &provisioning.Update{},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo",
			idArg:            uuid.MustParse(`13595731-843c-441e-9cf3-6c2869624cc8`),
			repoGetByUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*provisioning.Update, error) {
					return tc.repoGetByUUIDUpdate, tc.repoGetByUUIDErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, nil, nil)

			// Run test
			update, err := updateSvc.GetByUUID(context.Background(), tc.idArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByUUIDUpdate, update)
		})
	}
}

func TestUpdateService_GetUpdateAllFiles(t *testing.T) {
	tests := []struct {
		name                string
		idArg               uuid.UUID
		repoGetByUUIDUpdate *provisioning.Update
		repoGetByUUIDErr    error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "success",
			idArg: uuid.MustParse(`13595731-843c-441e-9cf3-6c2869624cc8`),
			repoGetByUUIDUpdate: &provisioning.Update{
				Files: provisioning.UpdateFiles{
					provisioning.UpdateFile{
						Filename: "dummy.txt",
						Size:     1,
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name:                "error - repo",
			idArg:               uuid.MustParse(`13595731-843c-441e-9cf3-6c2869624cc8`),
			repoGetByUUIDErr:    boom.Error,
			repoGetByUUIDUpdate: &provisioning.Update{},

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*provisioning.Update, error) {
					return tc.repoGetByUUIDUpdate, tc.repoGetByUUIDErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, nil, nil)

			// Run test
			updateFiles, err := updateSvc.GetUpdateAllFiles(context.Background(), tc.idArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByUUIDUpdate.Files, updateFiles)
		})
	}
}

func TestUpdateService_GetUpdateFileByFilename(t *testing.T) {
	tests := []struct {
		name                         string
		idArg                        uuid.UUID
		repoGetByUUIDUpdate          *provisioning.Update
		repoGetByUUIDErr             error
		repoUpdateFilesGetReadCloser io.ReadCloser
		repoUpdateFilesGetSize       int
		repoUpdateFilesGetErr        error

		assertErr require.ErrorAssertionFunc
		wantBody  []byte
		wantSize  int
	}{
		{
			name:  "success",
			idArg: uuid.MustParse(`13595731-843c-441e-9cf3-6c2869624cc8`),
			repoGetByUUIDUpdate: &provisioning.Update{
				Origin: "mock",
				Files: provisioning.UpdateFiles{
					provisioning.UpdateFile{
						Filename: "foo.bar",
					},
				},
			},
			repoUpdateFilesGetReadCloser: io.NopCloser(bytes.NewBuffer([]byte("foobar"))),
			repoUpdateFilesGetSize:       6,

			assertErr: require.NoError,
			wantBody:  []byte("foobar"),
			wantSize:  6,
		},
		{
			name:             "error - repo",
			idArg:            uuid.MustParse(`13595731-843c-441e-9cf3-6c2869624cc8`),
			repoGetByUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
			wantBody:  []byte{},
		},
		{
			name:  "error - file not found",
			idArg: uuid.MustParse(`13595731-843c-441e-9cf3-6c2869624cc8`),
			repoGetByUUIDUpdate: &provisioning.Update{
				Files: provisioning.UpdateFiles{}, // foo.bar not included
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Requested file "foo.bar" is not part of update`)
			},
			wantBody: []byte{},
		},
		{
			name:  "error - source",
			idArg: uuid.MustParse(`13595731-843c-441e-9cf3-6c2869624cc8`),
			repoGetByUUIDUpdate: &provisioning.Update{
				Origin: "mock",
				Files: provisioning.UpdateFiles{
					provisioning.UpdateFile{
						Filename: "foo.bar",
					},
				},
			},
			repoUpdateFilesGetReadCloser: io.NopCloser(bytes.NewBuffer([]byte{})),
			repoUpdateFilesGetErr:        boom.Error,

			assertErr: boom.ErrorIs,
			wantBody:  []byte{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*provisioning.Update, error) {
					return tc.repoGetByUUIDUpdate, tc.repoGetByUUIDErr
				},
			}

			repoUpdateFiles := &repoMock.UpdateFilesRepoMock{
				GetFunc: func(ctx context.Context, update provisioning.Update, filename string) (io.ReadCloser, int, error) {
					return tc.repoUpdateFilesGetReadCloser, tc.repoUpdateFilesGetSize, tc.repoUpdateFilesGetErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, repoUpdateFiles, nil)

			// Run test
			rc, size, err := updateSvc.GetUpdateFileByFilename(context.Background(), tc.idArg, "foo.bar")

			// Assert
			tc.assertErr(t, err)
			if rc != nil {
				defer rc.Close()
				body, err := io.ReadAll(rc)

				require.NoError(t, err)
				require.Equal(t, tc.wantBody, body)
				require.Equal(t, tc.wantSize, size)
			}
		})
	}
}

func TestUpdateService_Refresh(t *testing.T) {
	updatePresentUUID := uuidgen.FromPattern(t, "01")
	updateNewUUID := uuidgen.FromPattern(t, "02")

	dateTime1 := time.Date(2025, 8, 21, 13, 4, 0, 0, time.UTC)
	dateTime2 := time.Date(2025, 8, 22, 13, 4, 0, 0, time.UTC)
	dateTime3 := time.Date(2025, 8, 23, 13, 4, 0, 0, time.UTC)

	tests := []struct {
		name                 string
		ctx                  context.Context
		filterExpression     string
		fileFilterExpression string

		repoGetAllUpdates provisioning.Updates
		repoGetAllErr     error
		repoUpsert        []queue.Item[struct{}]
		repoDeleteByUUID  []queue.Item[struct{}]

		repoUpdateFilesUsageInformation []queue.Item[provisioning.UsageInformation]
		repoUpdateFilesPut              []queue.Item[struct {
			commitErr error
			cancelErr error
		}]
		repoUpdateFilesDelete []queue.Item[struct{}]

		sourceGetLatestUpdates        provisioning.Updates
		sourceGetLatestErr            error
		sourceGetUpdateFileByFilename []queue.Item[struct {
			stream io.ReadCloser
			size   int
		}]

		assertErr require.ErrorAssertionFunc
	}{
		// Success cases
		{
			name: "success - no updates, no state in the DB",
			ctx:  context.Background(),

			assertErr: require.NoError,
		},
		{
			name:             "success - one update, filtered",
			ctx:              context.Background(),
			filterExpression: "'stable' in Channels",

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Channels: provisioning.UpdateChannels{
						"daily",
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name: "success - one update, already present in DB",
			ctx:  context.Background(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
				},
			},
			repoGetAllUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime1,
				},
			},

			assertErr: require.NoError,
		},
		{
			name: "success - enhanced example",
			// Update source presents two updates.
			// One update is filtered based on filter expression and therefore skipped.
			// The other update is not present. It consists of two files, from which
			// one is filtered because of file filter for architecture.
			// The file, which is downloaded has a valid sha256 checksum, one file is
			// filtered.
			ctx:                  context.Background(),
			filterExpression:     "'stable' in Channels",
			fileFilterExpression: "'x86_64' == string(Architecture)",

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updateNewUUID,
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
					Severity:    api.UpdateSeverityNone,
					Channels: provisioning.UpdateChannels{
						"stable",
					},
					Files: provisioning.UpdateFiles{
						{
							Size: 5,

							// Generate hash: echo -n "dummy" | sha256sum
							Sha256: "b5a2c96250612366ea272ffac6d9744aaf4b45aacd96aa7cfcb931ee3b558259",

							Architecture: api.Architecture64BitIntelX86,
						},
						{
							// This file is filtered because of architecture.
							Size:         5,
							Architecture: api.Architecture64BitARMV8LittleEndian,
						},
					},
				},
				{
					UUID:        updateNewUUID,
					PublishedAt: dateTime3,
					Status:      api.UpdateStatusUnknown,
					Severity:    api.UpdateSeverityNone,
					Channels: provisioning.UpdateChannels{
						"daily", // This update is filtered based on filter expression
					},
					Files: provisioning.UpdateFiles{
						{
							Size: 5,
						},
					},
				},
			},
			repoGetAllUpdates: provisioning.Updates{},
			repoUpdateFilesUsageInformation: []queue.Item[provisioning.UsageInformation]{
				// global check
				{
					Value: usageInfoGiB(50, 10),
				},
				// 1st per update check
				{
					Value: usageInfoGiB(50, 10),
				},
			},
			repoUpsert: []queue.Item[struct{}]{
				// pending
				{},
				// ready
				{},
			},

			sourceGetUpdateFileByFilename: []queue.Item[struct {
				stream io.ReadCloser
				size   int
			}]{
				{
					Value: struct {
						stream io.ReadCloser
						size   int
					}{
						stream: io.NopCloser(bytes.NewBufferString(`dummy`)),
						size:   5,
					},
				},
			},
			repoUpdateFilesPut: []queue.Item[struct {
				commitErr error
				cancelErr error
			}]{
				// Finally one file is stored.
				{},
			},

			assertErr: require.NoError,
		},
		{
			name: "success - one update, which gets omitted, cleanup state in DB",
			ctx:  context.Background(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updateNewUUID,
					Status:      api.UpdateStatusUnknown,
					PublishedAt: dateTime3, // most recent update, but we always keep the most recent update from the DB and the test is configurued to only keep 1 update, so this gets omitted.
				},
			},
			repoGetAllUpdates: provisioning.Updates{
				{
					UUID:        uuidgen.FromPattern(t, "03"),
					Status:      api.UpdateStatusReady,
					PublishedAt: dateTime1, // delete, since it is the older one.
				},
				{
					UUID:        uuidgen.FromPattern(t, "04"),
					Status:      api.UpdateStatusReady,
					PublishedAt: dateTime3,
				},
				{
					UUID:        uuidgen.FromPattern(t, "05"),
					Status:      api.UpdateStatusPending,
					PublishedAt: dateTime3, // delete, since it is in pending for longer than grace period.
				},
			},
			repoUpdateFilesDelete: []queue.Item[struct{}]{
				{},
				{},
			},
			repoDeleteByUUID: []queue.Item[struct{}]{
				{},
				{},
			},

			assertErr: require.NoError,
		},

		// Error cases
		{
			name: "error - source.GetLatest",
			ctx:  context.Background(),

			sourceGetLatestErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:             "error - invalid filter expression",
			ctx:              context.Background(),
			filterExpression: "%", // invalid expression.

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Channels: provisioning.UpdateChannels{
						"daily",
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to compile filter expression")
			},
		},
		{
			name:             "error - filter expression run",
			ctx:              context.Background(),
			filterExpression: `fromBase64("~invalid")`, // invalid, returns runtime error during evauluation of the expression.

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Channels: provisioning.UpdateChannels{
						"daily",
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "illegal base64 data")
			},
		},
		{
			name:             "error - filter expression run",
			ctx:              context.Background(),
			filterExpression: `"string"`, // invalid, does evaluate to string instead of boolean.

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Channels: provisioning.UpdateChannels{
						"daily",
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "does not evaluate to boolean result")
			},
		},
		{
			name:                 "error - invalid file filter expression",
			ctx:                  context.Background(),
			fileFilterExpression: "%", // invalid expression.

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Files: provisioning.UpdateFiles{
						{
							Architecture: "x86_64",
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to compile file filter expression")
			},
		},
		{
			name:                 "error - file filter expression run",
			ctx:                  context.Background(),
			fileFilterExpression: `fromBase64("~invalid")`, // invalid, returns runtime error during evauluation of the expression.

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Files: provisioning.UpdateFiles{
						{
							Architecture: "x86_64",
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "illegal base64 data")
			},
		},
		{
			name:                 "error - file filter expression run",
			ctx:                  context.Background(),
			fileFilterExpression: `"string"`, // invalid, does evaluate to string instead of boolean.

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Files: provisioning.UpdateFiles{
						{
							Architecture: "x86_64",
						},
					},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "does not evaluate to boolean result")
			},
		},
		{
			name: "error - repo.GetAllWithFilter",
			ctx:  context.Background(),

			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - filesRepo.Delete",
			ctx:  context.Background(),

			repoGetAllUpdates: provisioning.Updates{
				{
					UUID:        uuidgen.FromPattern(t, "01"),
					Status:      api.UpdateStatusReady,
					PublishedAt: dateTime2,
				},
				{
					UUID:        uuidgen.FromPattern(t, "02"),
					Status:      api.UpdateStatusReady,
					PublishedAt: dateTime3,
				},
			},
			repoUpdateFilesDelete: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - filesRepo.Delete",
			ctx:  context.Background(),

			repoGetAllUpdates: provisioning.Updates{
				{
					UUID:        uuidgen.FromPattern(t, "01"),
					Status:      api.UpdateStatusReady,
					PublishedAt: dateTime2,
				},
				{
					UUID:        uuidgen.FromPattern(t, "02"),
					Status:      api.UpdateStatusReady,
					PublishedAt: dateTime3,
				},
			},
			repoUpdateFilesDelete: []queue.Item[struct{}]{
				{},
			},
			repoDeleteByUUID: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - filesRepo.UsageInformation",
			ctx:  context.Background(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
					Severity:    api.UpdateSeverityNone,
					Files: provisioning.UpdateFiles{
						{
							Size: 5,
						},
					},
				},
			},
			repoGetAllUpdates: provisioning.Updates{},
			repoUpdateFilesUsageInformation: []queue.Item[provisioning.UsageInformation]{
				// global check
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - filesRepo.UsageInformation - invalid total size",
			ctx:  context.Background(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
					Severity:    api.UpdateSeverityNone,
					Files: provisioning.UpdateFiles{
						{
							Size: 5,
						},
					},
				},
			},
			repoGetAllUpdates: provisioning.Updates{},
			repoUpdateFilesUsageInformation: []queue.Item[provisioning.UsageInformation]{
				// global check
				{
					Value: usageInfoGiB(0, 0), // invalid total size
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Files repository reported an invalid total space: 0")
			},
		},
		{
			name: "error - not enough space available global",
			ctx:  context.Background(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
					Severity:    api.UpdateSeverityNone,
					Files: provisioning.UpdateFiles{
						{
							Size: 5,
						},
					},
				},
			},
			repoGetAllUpdates: provisioning.Updates{},
			repoUpdateFilesUsageInformation: []queue.Item[provisioning.UsageInformation]{
				// global check
				{
					Value: usageInfoGiB(50, 0), // no space available
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Not enough space available in files repository")
			},
		},
		{
			name: "error - Validate",
			ctx:  context.Background(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
					Severity:    "invalid", // invalid
					Files: provisioning.UpdateFiles{
						{
							Size: 5,
						},
					},
				},
			},
			repoGetAllUpdates: provisioning.Updates{},
			repoUpdateFilesUsageInformation: []queue.Item[provisioning.UsageInformation]{
				// global check
				{
					Value: usageInfoGiB(50, 10),
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - repo.Upsert pending",
			ctx:  context.Background(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
					Severity:    api.UpdateSeverityNone,
					Files: provisioning.UpdateFiles{
						{
							Size: 5,
						},
					},
				},
			},
			repoGetAllUpdates: provisioning.Updates{},
			repoUpdateFilesUsageInformation: []queue.Item[provisioning.UsageInformation]{
				// global check
				{
					Value: usageInfoGiB(50, 10),
				},
			},
			repoUpsert: []queue.Item[struct{}]{
				// pending
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - not enough space available before download",
			ctx:  context.Background(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
					Severity:    api.UpdateSeverityNone,
					Files: provisioning.UpdateFiles{
						{
							Size: 5,
						},
					},
				},
			},
			repoGetAllUpdates: provisioning.Updates{},
			repoUpdateFilesUsageInformation: []queue.Item[provisioning.UsageInformation]{
				// global check
				{
					Value: usageInfoGiB(50, 10),
				},
				// 1st per update check
				{
					Value: usageInfoGiB(50, 0), // All space consumed
				},
			},
			repoUpsert: []queue.Item[struct{}]{
				// pending
				{},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Not enough space available in files repository")
			},
		},
		{
			name: "error - context cancelled",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancelCause(context.Background())
				cancel(boom.Error)
				return ctx
			}(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
					Severity:    api.UpdateSeverityNone,
					Files: provisioning.UpdateFiles{
						{
							Size: 5,
						},
					},
				},
			},
			repoGetAllUpdates: provisioning.Updates{},
			repoUpdateFilesUsageInformation: []queue.Item[provisioning.UsageInformation]{
				// global check
				{
					Value: usageInfoGiB(50, 10),
				},
				// 1st per update check
				{
					Value: usageInfoGiB(50, 10),
				},
			},
			repoUpsert: []queue.Item[struct{}]{
				// pending
				{},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - source.GetUpdateFileByFilename",
			ctx:  context.Background(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
					Severity:    api.UpdateSeverityNone,
					Files: provisioning.UpdateFiles{
						{
							Size: 5,
						},
					},
				},
			},
			repoGetAllUpdates: provisioning.Updates{},
			repoUpdateFilesUsageInformation: []queue.Item[provisioning.UsageInformation]{
				// global check
				{
					Value: usageInfoGiB(50, 10),
				},
				// 1st per update check
				{
					Value: usageInfoGiB(50, 10),
				},
			},
			repoUpsert: []queue.Item[struct{}]{
				// pending
				{},
			},
			sourceGetUpdateFileByFilename: []queue.Item[struct {
				stream io.ReadCloser
				size   int
			}]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - filesRepo.Put",
			ctx:  context.Background(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
					Severity:    api.UpdateSeverityNone,
					Files: provisioning.UpdateFiles{
						{
							Size: 5,

							// Generate hash: echo -n "dummy" | sha256sum
							Sha256: "b5a2c96250612366ea272ffac6d9744aaf4b45aacd96aa7cfcb931ee3b558259",
						},
					},
				},
			},
			repoGetAllUpdates: provisioning.Updates{},
			repoUpdateFilesUsageInformation: []queue.Item[provisioning.UsageInformation]{
				// global check
				{
					Value: usageInfoGiB(50, 10),
				},
				// 1st per update check
				{
					Value: usageInfoGiB(50, 10),
				},
			},
			repoUpsert: []queue.Item[struct{}]{
				// pending
				{},
			},
			sourceGetUpdateFileByFilename: []queue.Item[struct {
				stream io.ReadCloser
				size   int
			}]{
				{
					Value: struct {
						stream io.ReadCloser
						size   int
					}{
						stream: io.NopCloser(bytes.NewBufferString(`dummy`)),
						size:   5,
					},
				},
			},
			repoUpdateFilesPut: []queue.Item[struct {
				commitErr error
				cancelErr error
			}]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - filesRepo.Put - invalid sha256",
			ctx:  context.Background(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
					Severity:    api.UpdateSeverityNone,
					Files: provisioning.UpdateFiles{
						{
							Size: 5,

							Sha256: "invalid", // invalid hash
						},
					},
				},
			},
			repoGetAllUpdates: provisioning.Updates{},
			repoUpdateFilesUsageInformation: []queue.Item[provisioning.UsageInformation]{
				// global check
				{
					Value: usageInfoGiB(50, 10),
				},
				// 1st per update check
				{
					Value: usageInfoGiB(50, 10),
				},
			},
			repoUpsert: []queue.Item[struct{}]{
				// pending
				{},
			},
			sourceGetUpdateFileByFilename: []queue.Item[struct {
				stream io.ReadCloser
				size   int
			}]{
				{
					Value: struct {
						stream io.ReadCloser
						size   int
					}{
						stream: io.NopCloser(bytes.NewBufferString(`dummy`)),
						size:   5,
					},
				},
			},
			repoUpdateFilesPut: []queue.Item[struct {
				commitErr error
				cancelErr error
			}]{
				{},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Invalid update, file sha256 mismatch for file")
			},
		},
		{
			name: "error - filesRepo.Put - commit",
			ctx:  context.Background(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
					Severity:    api.UpdateSeverityNone,
					Files: provisioning.UpdateFiles{
						{
							Size: 5,
						},
					},
				},
			},
			repoGetAllUpdates: provisioning.Updates{},
			repoUpdateFilesUsageInformation: []queue.Item[provisioning.UsageInformation]{
				// global check
				{
					Value: usageInfoGiB(50, 10),
				},
				// 1st per update check
				{
					Value: usageInfoGiB(50, 10),
				},
			},
			repoUpsert: []queue.Item[struct{}]{
				// pending
				{},
			},
			sourceGetUpdateFileByFilename: []queue.Item[struct {
				stream io.ReadCloser
				size   int
			}]{
				{
					Value: struct {
						stream io.ReadCloser
						size   int
					}{
						stream: io.NopCloser(bytes.NewBufferString(`dummy`)),
						size:   5,
					},
				},
			},
			repoUpdateFilesPut: []queue.Item[struct {
				commitErr error
				cancelErr error
			}]{
				{
					Value: struct {
						commitErr error
						cancelErr error
					}{
						commitErr: boom.Error,
					},
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - filesRepo.Put - cancel",
			ctx:  context.Background(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
					Severity:    api.UpdateSeverityNone,
					Files: provisioning.UpdateFiles{
						{
							Size: 5,
						},
					},
				},
			},
			repoGetAllUpdates: provisioning.Updates{},
			repoUpdateFilesUsageInformation: []queue.Item[provisioning.UsageInformation]{
				// global check
				{
					Value: usageInfoGiB(50, 10),
				},
				// 1st per update check
				{
					Value: usageInfoGiB(50, 10),
				},
			},
			repoUpsert: []queue.Item[struct{}]{
				// pending
				{},
			},
			sourceGetUpdateFileByFilename: []queue.Item[struct {
				stream io.ReadCloser
				size   int
			}]{
				{
					Value: struct {
						stream io.ReadCloser
						size   int
					}{
						stream: io.NopCloser(bytes.NewBufferString(`dummy`)),
						size:   5,
					},
				},
			},
			repoUpdateFilesPut: []queue.Item[struct {
				commitErr error
				cancelErr error
			}]{
				{
					Value: struct {
						commitErr error
						cancelErr error
					}{
						cancelErr: boom.Error,
					},
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - filesRepo.Upsert",
			ctx:  context.Background(),

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID:        updatePresentUUID,
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
					Severity:    api.UpdateSeverityNone,
					Files: provisioning.UpdateFiles{
						{
							Size: 5,
						},
					},
				},
			},
			repoGetAllUpdates: provisioning.Updates{},
			repoUpdateFilesUsageInformation: []queue.Item[provisioning.UsageInformation]{
				// global check
				{
					Value: usageInfoGiB(50, 10),
				},
				// 1st per update check
				{
					Value: usageInfoGiB(50, 10),
				},
			},
			repoUpsert: []queue.Item[struct{}]{
				// pending
				{},
				// ready
				{
					Err: boom.Error,
				},
			},
			sourceGetUpdateFileByFilename: []queue.Item[struct {
				stream io.ReadCloser
				size   int
			}]{
				{
					Value: struct {
						stream io.ReadCloser
						size   int
					}{
						stream: io.NopCloser(bytes.NewBufferString(`dummy`)),
						size:   5,
					},
				},
			},
			repoUpdateFilesPut: []queue.Item[struct {
				commitErr error
				cancelErr error
			}]{
				{},
			},

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// if tc.name != "error - filesRepo.UsageInformation - invalid total size" {
			// 	t.SkipNow()
			// }

			// Setup
			repo := &repoMock.UpdateRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Updates, error) {
					return tc.repoGetAllUpdates, tc.repoGetAllErr
				},
				UpsertFunc: func(ctx context.Context, update provisioning.Update) error {
					_, err := queue.Pop(t, &tc.repoUpsert)
					return err
				},
				DeleteByUUIDFunc: func(ctx context.Context, id uuid.UUID) error {
					_, err := queue.Pop(t, &tc.repoDeleteByUUID)
					return err
				},
			}

			repoUpdateFiles := &repoMock.UpdateFilesRepoMock{
				PutFunc: func(ctx context.Context, update provisioning.Update, filename string, content io.ReadCloser) (provisioning.CommitFunc, provisioning.CancelFunc, error) {
					_, err := io.ReadAll(content)
					require.NoError(t, err)

					value, err := queue.Pop(t, &tc.repoUpdateFilesPut)

					commitFunc := func() error { return value.commitErr }
					cancelFunc := func() error { return value.cancelErr }

					return commitFunc, cancelFunc, err
				},
				DeleteFunc: func(ctx context.Context, update provisioning.Update) error {
					_, err := queue.Pop(t, &tc.repoUpdateFilesDelete)
					return err
				},
				UsageInformationFunc: func(ctx context.Context) (provisioning.UsageInformation, error) {
					return queue.Pop(t, &tc.repoUpdateFilesUsageInformation)
				},
			}

			source := &adapterMock.UpdateSourcePortMock{
				GetLatestFunc: func(ctx context.Context, limit int) (provisioning.Updates, error) {
					return tc.sourceGetLatestUpdates, tc.sourceGetLatestErr
				},
				GetUpdateFileByFilenameUnverifiedFunc: func(ctx context.Context, update provisioning.Update, filename string) (io.ReadCloser, int, error) {
					value, err := queue.Pop(t, &tc.sourceGetUpdateFileByFilename)
					return value.stream, value.size, err
				},
			}

			updateSvc := provisioning.NewUpdateService(
				repo,
				repoUpdateFiles,
				source,
				provisioning.UpdateServiceWithLatestLimit(1),
				provisioning.UpdateServiceWithPendingGracePeriod(24*time.Hour),
				provisioning.UpdateServiceWithFilterExpression(tc.filterExpression),
				provisioning.UpdateServiceWithFileFilterExpression(tc.fileFilterExpression),
			)

			// Run test
			err := updateSvc.Refresh(tc.ctx)

			// Assert
			tc.assertErr(t, err)

			// Ensure queues are completely drained.
			require.Empty(t, tc.repoUpsert)
			require.Empty(t, tc.repoDeleteByUUID)
			require.Empty(t, tc.repoUpdateFilesPut)
			require.Empty(t, tc.repoUpdateFilesDelete)
			require.Empty(t, tc.sourceGetUpdateFileByFilename)
		})
	}
}

func usageInfoGiB(totalSpaceGiB int, availableSpaceGiB int) provisioning.UsageInformation {
	const GiB = 1024 * 1024 * 1024
	return provisioning.UsageInformation{
		TotalSpaceBytes:     uint64(totalSpaceGiB) * GiB,
		AvailableSpaceBytes: uint64(availableSpaceGiB) * GiB,
		UsedSpaceBytes:      uint64(totalSpaceGiB-availableSpaceGiB) * GiB,
	}
}
