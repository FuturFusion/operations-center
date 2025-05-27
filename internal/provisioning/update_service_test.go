package provisioning_test

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"testing"
	"testing/iotest"
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

		sourceAddUUID      uuid.UUID
		sourceAddErr       error
		sourceGetLatestErr error

		assertErr require.ErrorAssertionFunc
		wantID    uuid.UUID
	}{
		{
			name: "success",

			sourceAddUUID: uuid.MustParse(`98e0ec84-eb21-4406-a7bf-727610d4d0c4`),

			assertErr: require.NoError,
			wantID:    uuid.MustParse(`98e0ec84-eb21-4406-a7bf-727610d4d0c4`),
		},
		{
			name: "error - source.GetLatest",

			sourceAddErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - source.GetLatest",

			sourceGetLatestErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &repoMock.UpdateRepoMock{
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.UpdateFilter) (provisioning.Updates, error) {
					return nil, nil
				},
			}

			source := &adapterMock.UpdateSourceWithForgetAndAddPortMock{
				AddFunc: func(ctx context.Context, tarReader *tar.Reader) (*provisioning.Update, error) {
					return &provisioning.Update{
						UUID: tc.sourceAddUUID,
					}, tc.sourceAddErr
				},
				GetLatestFunc: func(ctx context.Context, limit int) (provisioning.Updates, error) {
					return nil, tc.sourceGetLatestErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, provisioning.UpdateServiceWithSource("mock", source))

			// Run test
			id, err := updateSvc.CreateFromArchive(context.Background(), nil)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantID, id)
		})
	}
}

func TestUpdateService_CreateFromArchive_NoSourceWithAdd(t *testing.T) {
	tests := []struct {
		name string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "error - no source with add",

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updateSvc := provisioning.NewUpdateService(nil)

			// Run test
			id, err := updateSvc.CreateFromArchive(context.Background(), nil)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, uuid.UUID{}, id)
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

			updateSvc := provisioning.NewUpdateService(repo)

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

			serverSvc := provisioning.NewUpdateService(repo)

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

			updateSvc := provisioning.NewUpdateService(repo)

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

			serverSvc := provisioning.NewUpdateService(repo)

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

			updateSvc := provisioning.NewUpdateService(repo)

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

			updateSvc := provisioning.NewUpdateService(repo)

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
		name                              string
		idArg                             uuid.UUID
		repoGetByUUIDUpdate               *provisioning.Update
		repoGetByUUIDErr                  error
		sourceGetUpdateFileByFilename     io.ReadCloser
		sourceGetUpdateFileByFilenameSize int
		sourceGetUpdateFileByFilenameErr  error

		assertErr require.ErrorAssertionFunc
		wantBody  []byte
		wantSize  int
	}{
		{
			name:  "success",
			idArg: uuid.MustParse(`13595731-843c-441e-9cf3-6c2869624cc8`),
			repoGetByUUIDUpdate: &provisioning.Update{
				Origin: "mock",
			},
			sourceGetUpdateFileByFilename:     io.NopCloser(bytes.NewBuffer([]byte("foobar"))),
			sourceGetUpdateFileByFilenameSize: 6,

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
			name:  "error - unsupported origin",
			idArg: uuid.MustParse(`13595731-843c-441e-9cf3-6c2869624cc8`),
			repoGetByUUIDUpdate: &provisioning.Update{
				Origin: "unsupported", // invalid
			},

			assertErr: require.Error,
			wantBody:  []byte{},
		},
		{
			name:  "error - source",
			idArg: uuid.MustParse(`13595731-843c-441e-9cf3-6c2869624cc8`),
			repoGetByUUIDUpdate: &provisioning.Update{
				Origin: "mock",
			},
			sourceGetUpdateFileByFilename:    io.NopCloser(bytes.NewBuffer([]byte{})),
			sourceGetUpdateFileByFilenameErr: boom.Error,

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

			source := &adapterMock.UpdateSourcePortMock{
				GetUpdateFileByFilenameFunc: func(ctx context.Context, update provisioning.Update, filename string) (io.ReadCloser, int, error) {
					return tc.sourceGetUpdateFileByFilename, tc.sourceGetUpdateFileByFilenameSize, tc.sourceGetUpdateFileByFilenameErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, provisioning.UpdateServiceWithSource("mock", source))

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
		sourceForgetUpdate []queue.Item[struct{}]

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
			sourceForgetUpdate: []queue.Item[struct{}]{
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
			name: "error - file download error",
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
						stream: io.NopCloser(iotest.ErrReader(boom.Error)),
						size:   5,
					},
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - file download close error",
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
						stream: func() io.ReadCloser {
							return errCloser(bytes.NewBufferString(``), boom.Error)
						}(),
						size: 5,
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
			sourceForgetUpdate: []queue.Item[struct{}]{
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
			sourceForgetUpdate: []queue.Item[struct{}]{
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

			source := &adapterMock.UpdateSourceWithForgetPortMock{
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
				ForgetUpdateFunc: func(ctx context.Context, update provisioning.Update) error {
					_, err := queue.Pop(t, &tc.sourceForgetUpdate)
					return err
				},
			}

			updateSvc := provisioning.NewUpdateService(repo, provisioning.UpdateServiceWithSource("mock", source), provisioning.UpdateServiceWithLatestLimit(1))

			// Run test
			err := updateSvc.Refresh(tc.ctx)

			// Assert
			tc.assertErr(t, err)

			// Ensure queues are completely drained.
			require.Empty(t, tc.sourceGetUpdateAllFiles)
			require.Empty(t, tc.sourceGetUpdateFileByFilename)
			require.Empty(t, tc.sourceForgetUpdate)
			require.Empty(t, tc.repoUpsert)
			require.Empty(t, tc.repoDeleteByUUID)
		})
	}
}

func errCloser(r io.Reader, err error) io.ReadCloser {
	return nopCloser{
		Reader: r,
		err:    err,
	}
}

type nopCloser struct {
	io.Reader

	err error
}

func (n nopCloser) Close() error { return n.err }
