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

	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
	"github.com/FuturFusion/operations-center/internal/testing/queue"
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
				UUID: uuid.MustParse(`98e0ec84-eb21-4406-a7bf-727610d4d0c4`),
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
			name: "error - Upsert",

			repoUpdateFilesCreateFromArchiveUpdate: &provisioning.Update{
				UUID: uuid.MustParse(`98e0ec84-eb21-4406-a7bf-727610d4d0c4`),
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

			updateSvc := provisioning.NewUpdateService(repo, repoUpdateFiles)

			// Run test
			id, err := updateSvc.CreateFromArchive(context.Background(), nil)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantID, id)
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

			updateSvc := provisioning.NewUpdateService(repo, nil)

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
			name: "success - no filter expression",
			filter: provisioning.UpdateFilter{
				Channel: ptr.To("one"),
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

			serverSvc := provisioning.NewUpdateService(repo, nil)

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

			updateSvc := provisioning.NewUpdateService(repo, nil)

			// Run test
			updates, err := updateSvc.GetAllUUIDs(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetAllUUIDs, updates)
		})
	}
}

func TestUpdateService_GetAllIDsWithFilter(t *testing.T) {
	tests := []struct {
		name                         string
		filter                       provisioning.UpdateFilter
		repoGetAllUUIDsWithFilter    []uuid.UUID
		repoGetAllUUIDsWithFilterErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success - no filter expression",
			filter: provisioning.UpdateFilter{
				Channel: ptr.To("one"),
			},
			repoGetAllUUIDsWithFilter: []uuid.UUID{
				uuid.MustParse(`8926daa1-3a48-4739-9a82-e32ebd22d343`),
				uuid.MustParse(`84156d67-0bcb-4b60-ac23-2c67f552fb8c`),
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:                         "error - repo",
			repoGetAllUUIDsWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				GetAllUUIDsFunc: func(ctx context.Context) ([]uuid.UUID, error) {
					return tc.repoGetAllUUIDsWithFilter, tc.repoGetAllUUIDsWithFilterErr
				},
				GetAllUUIDsWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) ([]uuid.UUID, error) {
					return tc.repoGetAllUUIDsWithFilter, tc.repoGetAllUUIDsWithFilterErr
				},
			}

			serverSvc := provisioning.NewUpdateService(repo, nil)

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

			updateSvc := provisioning.NewUpdateService(repo, nil)

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
						URL:      "https://localhost/dummy.txt",
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

			updateSvc := provisioning.NewUpdateService(repo, nil)

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

			assertErr: func(tt require.TestingT, err error, i ...any) {
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

			updateSvc := provisioning.NewUpdateService(repo, repoUpdateFiles)

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
	tests := []struct {
		name string
		ctx  context.Context

		sourceGetLatestUpdates        provisioning.Updates
		sourceGetLatestErr            error
		sourceGetUpdateAllFiles       []queue.Item[provisioning.UpdateFiles]
		sourceGetUpdateFileByFilename []queue.Item[struct {
			stream io.ReadCloser
			size   int
		}]
		repoUpdateFilesPut []queue.Item[struct {
			commitErr error
			cancelErr error
		}]
		repoUpdateFilesDelete []queue.Item[struct{}]

		repoUpsert                  []queue.Item[struct{}]
		repoGetAllWithFilterUpdates provisioning.Updates
		repoGetAllWithFilterErr     error
		repoDeleteByUUID            []queue.Item[struct{}]

		assertErr require.ErrorAssertionFunc
	}{
		// Success cases
		{
			name: "success - no updates, no state in the DB",
			ctx:  context.Background(),

			assertErr: require.NoError,
		},
		{
			name: "success - one update, not present, with files",
			ctx:  context.Background(),
			sourceGetLatestUpdates: provisioning.Updates{
				provisioning.Update{},
			},
			sourceGetUpdateAllFiles: []queue.Item[provisioning.UpdateFiles]{
				{
					Value: provisioning.UpdateFiles{
						provisioning.UpdateFile{
							Filename: "dummy.txt",
							URL:      "http://localhost/dummy.txt",
							Size:     5,
						},
					},
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
			repoUpsert: []queue.Item[struct{}]{
				{},
			},

			assertErr: require.NoError,
		},
		{
			name: "success - one update, not present, with files and sha256 checksum",
			ctx:  context.Background(),
			sourceGetLatestUpdates: provisioning.Updates{
				provisioning.Update{},
			},
			sourceGetUpdateAllFiles: []queue.Item[provisioning.UpdateFiles]{
				{
					Value: provisioning.UpdateFiles{
						provisioning.UpdateFile{
							Filename: "dummy.txt",
							URL:      "http://localhost/dummy.txt",
							Size:     5,

							// Generate hash: echo -n "dummy" | sha256sum
							Sha256: "b5a2c96250612366ea272ffac6d9744aaf4b45aacd96aa7cfcb931ee3b558259",
						},
					},
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
			repoUpsert: []queue.Item[struct{}]{
				{},
			},

			assertErr: require.NoError,
		},
		{
			name: "success - no updates, cleanup state in DB",
			ctx:  context.Background(),
			repoGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID:        uuid.MustParse(`223795ef-a126-4e91-8d19-9d550ff928d6`),
					PublishedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				{
					UUID:        uuid.MustParse(`af49c1b9-4fdf-4542-a113-456316d045f4`),
					PublishedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
				},
			},
			repoUpdateFilesDelete: []queue.Item[struct{}]{
				{},
			},
			repoDeleteByUUID: []queue.Item[struct{}]{
				{},
			},

			assertErr: require.NoError,
		},

		// Error cases
		{
			name:               "error - source.GetLatest",
			ctx:                context.Background(),
			sourceGetLatestErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - source.GetUpdateAllFiles",
			ctx:  context.Background(),
			sourceGetLatestUpdates: provisioning.Updates{
				provisioning.Update{},
			},
			sourceGetUpdateAllFiles: []queue.Item[provisioning.UpdateFiles]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - context cancelled",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancelCause(context.Background())
				cancel(boom.Error)
				return ctx
			}(),
			sourceGetLatestUpdates: provisioning.Updates{
				provisioning.Update{},
			},
			sourceGetUpdateAllFiles: []queue.Item[provisioning.UpdateFiles]{
				{
					Value: provisioning.UpdateFiles{
						provisioning.UpdateFile{
							Filename: "dummy.txt",
							URL:      "http://localhost/dummy.txt",
							Size:     5,
						},
					},
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - source.GetUpdateFileByFilename",
			ctx:  context.Background(),
			sourceGetLatestUpdates: provisioning.Updates{
				provisioning.Update{},
			},
			sourceGetUpdateAllFiles: []queue.Item[provisioning.UpdateFiles]{
				{
					Value: provisioning.UpdateFiles{
						provisioning.UpdateFile{
							Filename: "dummy.txt",
							URL:      "http://localhost/dummy.txt",
							Size:     5,
						},
					},
				},
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
			name: "error - files repo Put",
			ctx:  context.Background(),
			sourceGetLatestUpdates: provisioning.Updates{
				provisioning.Update{},
			},
			sourceGetUpdateAllFiles: []queue.Item[provisioning.UpdateFiles]{
				{
					Value: provisioning.UpdateFiles{
						provisioning.UpdateFile{
							Filename: "dummy.txt",
							URL:      "http://localhost/dummy.txt",
							Size:     5,
						},
					},
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
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - checksum error",
			ctx:  context.Background(),
			sourceGetLatestUpdates: provisioning.Updates{
				provisioning.Update{},
			},
			sourceGetUpdateAllFiles: []queue.Item[provisioning.UpdateFiles]{
				{
					Value: provisioning.UpdateFiles{
						provisioning.UpdateFile{
							Filename: "dummy.txt",
							URL:      "http://localhost/dummy.txt",
							Size:     5,

							Sha256: "invalid", // invalid hash
						},
					},
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

			assertErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorContains(tt, err, "Invalid update, file sha256 mismatch for file")
			},
		},
		{
			name: "error - commit",
			ctx:  context.Background(),
			sourceGetLatestUpdates: provisioning.Updates{
				provisioning.Update{},
			},
			sourceGetUpdateAllFiles: []queue.Item[provisioning.UpdateFiles]{
				{
					Value: provisioning.UpdateFiles{
						provisioning.UpdateFile{
							Filename: "dummy.txt",
							URL:      "http://localhost/dummy.txt",
							Size:     5,
						},
					},
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
			name: "error - cancel",
			ctx:  context.Background(),
			sourceGetLatestUpdates: provisioning.Updates{
				provisioning.Update{},
			},
			sourceGetUpdateAllFiles: []queue.Item[provisioning.UpdateFiles]{
				{
					Value: provisioning.UpdateFiles{
						provisioning.UpdateFile{
							Filename: "dummy.txt",
							URL:      "http://localhost/dummy.txt",
							Size:     5,
						},
					},
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
			name: "error - repo.Upsert",
			ctx:  context.Background(),
			sourceGetLatestUpdates: provisioning.Updates{
				provisioning.Update{},
			},
			sourceGetUpdateAllFiles: []queue.Item[provisioning.UpdateFiles]{
				{
					Value: provisioning.UpdateFiles{
						provisioning.UpdateFile{
							Filename: "dummy.txt",
							URL:      "http://localhost/dummy.txt",
							Size:     5,
						},
					},
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
			repoUpsert: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name:                    "error - repo.GetAll",
			ctx:                     context.Background(),
			repoGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - source.ForgetUpdate",
			ctx:  context.Background(),
			repoGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID:        uuid.MustParse(`223795ef-a126-4e91-8d19-9d550ff928d6`),
					PublishedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				{
					UUID:        uuid.MustParse(`af49c1b9-4fdf-4542-a113-456316d045f4`),
					PublishedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
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
			name: "error - repo.DeleteByUUID",
			ctx:  context.Background(),
			repoGetAllWithFilterUpdates: provisioning.Updates{
				{
					UUID:        uuid.MustParse(`223795ef-a126-4e91-8d19-9d550ff928d6`),
					PublishedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				{
					UUID:        uuid.MustParse(`af49c1b9-4fdf-4542-a113-456316d045f4`),
					PublishedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return tc.repoGetAllWithFilterUpdates, tc.repoGetAllWithFilterErr
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
			}

			source := &adapterMock.UpdateSourcePortMock{
				GetLatestFunc: func(ctx context.Context, limit int) (provisioning.Updates, error) {
					return tc.sourceGetLatestUpdates, tc.sourceGetLatestErr
				},
				GetUpdateAllFilesFunc: func(ctx context.Context, update provisioning.Update) (provisioning.UpdateFiles, error) {
					return queue.Pop(t, &tc.sourceGetUpdateAllFiles)
				},
				GetUpdateFileByFilenameFunc: func(ctx context.Context, update provisioning.Update, filename string) (io.ReadCloser, int, error) {
					value, err := queue.Pop(t, &tc.sourceGetUpdateFileByFilename)
					return value.stream, value.size, err
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, repoUpdateFiles, provisioning.UpdateServiceWithSource("mock", source), provisioning.UpdateServiceWithLatestLimit(1))

			// Run test
			err := updateSvc.Refresh(tc.ctx)

			// Assert
			tc.assertErr(t, err)

			// Ensure queues are completely drained.
			require.Empty(t, tc.sourceGetUpdateAllFiles)
			require.Empty(t, tc.sourceGetUpdateFileByFilename)
			require.Empty(t, tc.repoUpdateFilesDelete)
			require.Empty(t, tc.repoUpsert)
			require.Empty(t, tc.repoDeleteByUUID)
		})
	}
}
