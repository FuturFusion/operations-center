package provisioning_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
)

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
			repo := &mock.UpdateRepoMock{
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

func TestUpdateService_GetAllIDs(t *testing.T) {
	tests := []struct {
		name             string
		repoGetAllIDs    []string
		repoGetAllIDsErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:          "success",
			repoGetAllIDs: []string{"foo", "bar"},

			assertErr: require.NoError,
		},
		{
			name:             "error - repo",
			repoGetAllIDsErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.UpdateRepoMock{
				GetAllIDsFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllIDs, tc.repoGetAllIDsErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo)

			// Run test
			updates, err := updateSvc.GetAllIDs(context.Background())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetAllIDs, updates)
		})
	}
}

func TestUpdateService_GetByID(t *testing.T) {
	tests := []struct {
		name              string
		idArg             string
		repoGetByIDUpdate provisioning.Update
		repoGetByIDErr    error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:              "success",
			idArg:             "foo",
			repoGetByIDUpdate: provisioning.Update{},

			assertErr: require.NoError,
		},
		{
			name:           "error - repo",
			idArg:          "foo",
			repoGetByIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.UpdateRepoMock{
				GetByIDFunc: func(ctx context.Context, id string) (provisioning.Update, error) {
					return tc.repoGetByIDUpdate, tc.repoGetByIDErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo)

			// Run test
			update, err := updateSvc.GetByID(context.Background(), tc.idArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByIDUpdate, update)
		})
	}
}

func TestUpdateService_GetUpdateAllFiles(t *testing.T) {
	tests := []struct {
		name                     string
		idArg                    string
		repoGetUpdateAllFiles    provisioning.UpdateFiles
		repoGetUpdateAllFilesErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                  "success",
			idArg:                 "foo",
			repoGetUpdateAllFiles: provisioning.UpdateFiles{},

			assertErr: require.NoError,
		},
		{
			name:                     "error - repo",
			idArg:                    "foo",
			repoGetUpdateAllFilesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.UpdateRepoMock{
				GetUpdateAllFilesFunc: func(ctx context.Context, updateID string) (provisioning.UpdateFiles, error) {
					return tc.repoGetUpdateAllFiles, tc.repoGetUpdateAllFilesErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo)

			// Run test
			updateFiles, err := updateSvc.GetUpdateAllFiles(context.Background(), tc.idArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetUpdateAllFiles, updateFiles)
		})
	}
}

func TestUpdateService_GetUpdateFileByFilename(t *testing.T) {
	tests := []struct {
		name                            string
		idArg                           string
		repoGetUpdateFileByFilename     io.ReadCloser
		repoGetUpdateFileByFilenameSize int
		repoGetUpdateFileByFilenameErr  error

		assertErr require.ErrorAssertionFunc
		wantBody  []byte
		wantSize  int
	}{
		{
			name:                            "success",
			idArg:                           "foo",
			repoGetUpdateFileByFilename:     io.NopCloser(bytes.NewBuffer([]byte("foobar"))),
			repoGetUpdateFileByFilenameSize: 6,

			assertErr: require.NoError,
			wantBody:  []byte("foobar"),
			wantSize:  6,
		},
		{
			name:                           "error - repo",
			idArg:                          "foo",
			repoGetUpdateFileByFilename:    io.NopCloser(bytes.NewBuffer([]byte{})),
			repoGetUpdateFileByFilenameErr: boom.Error,

			assertErr: boom.ErrorIs,
			wantBody:  []byte{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.UpdateRepoMock{
				GetUpdateFileByFilenameFunc: func(ctx context.Context, updateID, filename string) (io.ReadCloser, int, error) {
					return tc.repoGetUpdateFileByFilename, tc.repoGetUpdateFileByFilenameSize, tc.repoGetUpdateFileByFilenameErr
				},
			}

			updateSvc := provisioning.NewUpdateService(repo)

			// Run test
			rc, size, err := updateSvc.GetUpdateFileByFilename(context.Background(), tc.idArg, "foo.bar")

			// Assert
			tc.assertErr(t, err)
			defer rc.Close()
			body, err := io.ReadAll(rc)
			require.NoError(t, err)
			require.Equal(t, tc.wantBody, body)
			require.Equal(t, tc.wantSize, size)
		})
	}
}
