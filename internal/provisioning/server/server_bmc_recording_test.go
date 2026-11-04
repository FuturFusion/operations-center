package server_test

import (
	"context"
	"crypto/tls"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	provisioningServer "github.com/FuturFusion/operations-center/internal/provisioning/server"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/shared/api"
)

const testBMCRecordingID = "20261027T081500Z"

// bmcRecordingCall runs one of the BMC recording methods of the service and
// returns the result in a common shape.
type bmcRecordingMethod func(ctx context.Context, svc provisioning.ServerService, name string, id string) (any, error)

var bmcRecordingMethods = map[string]bmcRecordingMethod{
	"GetBMCRecordings": func(ctx context.Context, svc provisioning.ServerService, name string, id string) (any, error) {
		return svc.GetBMCRecordings(ctx, name)
	},
	"GetBMCRecording": func(ctx context.Context, svc provisioning.ServerService, name string, id string) (any, error) {
		return svc.GetBMCRecording(ctx, name, id)
	},
	"GetBMCRecordingArchive": func(ctx context.Context, svc provisioning.ServerService, name string, id string) (any, error) {
		rc, size, err := svc.GetBMCRecordingArchive(ctx, name, id, false)
		return readBMCRecordingBody(rc, int64(size), err)
	},
	"GetBMCRecordingArchiveAnonymized": func(ctx context.Context, svc provisioning.ServerService, name string, id string) (any, error) {
		rc, size, err := svc.GetBMCRecordingArchive(ctx, name, id, true)
		return readBMCRecordingBody(rc, int64(size), err)
	},
	"GetBMCRecordingFile": func(ctx context.Context, svc provisioning.ServerService, name string, id string) (any, error) {
		rc, size, err := svc.GetBMCRecordingFile(ctx, name, id, "manifest.json")
		return readBMCRecordingBody(rc, size, err)
	},
	"DeleteBMCRecording": func(ctx context.Context, svc provisioning.ServerService, name string, id string) (any, error) {
		return nil, svc.DeleteBMCRecording(ctx, name, id)
	},
}

func readBMCRecordingBody(rc io.ReadCloser, size int64, err error) (any, error) {
	if err != nil {
		return nil, err
	}

	defer rc.Close()

	body, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	return map[string]any{"body": string(body), "size": size}, nil
}

func newBMCRecordingRepoMock(err error) *repoMock.BMCRecordingRepoMock {
	return &repoMock.BMCRecordingRepoMock{
		GetAllFunc: func(ctx context.Context, serverName string) ([]api.BMCRecordingManifest, error) {
			return []api.BMCRecordingManifest{{Server: serverName, RecordingID: testBMCRecordingID}}, err
		},
		GetByIDFunc: func(ctx context.Context, serverName string, id string) (*api.BMCRecordingManifest, error) {
			return &api.BMCRecordingManifest{Server: serverName, RecordingID: id}, err
		},
		GetArchiveFunc: func(ctx context.Context, serverName string, id string, rewrite *provisioning.BMCRecordingRewrite) (io.ReadCloser, int, error) {
			return io.NopCloser(strings.NewReader("zip")), 3, err
		},
		GetFileFunc: func(ctx context.Context, serverName string, id string, filename string) (io.ReadCloser, int64, error) {
			return io.NopCloser(strings.NewReader(filename)), int64(len(filename)), err
		},
		DeleteByIDFunc: func(ctx context.Context, serverName string, id string) error {
			return err
		},
	}
}

func TestServerService_BMCRecording(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		method           string
		nameArg          string
		idArg            string
		repoGetByNameErr error
		recordingErr     error

		assertErr   require.ErrorAssertionFunc
		want        any
		wantCalls   int
		wantRewrite []*provisioning.BMCRecordingRewrite
	}{
		{
			name:    "success - GetBMCRecordings",
			method:  "GetBMCRecordings",
			nameArg: "one",

			assertErr: require.NoError,
			want:      []api.BMCRecordingManifest{{Server: "one", RecordingID: testBMCRecordingID}},
			wantCalls: 1,
		},
		{
			name:    "success - GetBMCRecording",
			method:  "GetBMCRecording",
			nameArg: "one",
			idArg:   testBMCRecordingID,

			assertErr: require.NoError,
			want:      &api.BMCRecordingManifest{Server: "one", RecordingID: testBMCRecordingID},
			wantCalls: 1,
		},
		{
			name:    "success - GetBMCRecordingArchive",
			method:  "GetBMCRecordingArchive",
			nameArg: "one",
			idArg:   testBMCRecordingID,

			assertErr:   require.NoError,
			want:        map[string]any{"body": "zip", "size": int64(3)},
			wantCalls:   1,
			wantRewrite: []*provisioning.BMCRecordingRewrite{nil},
		},
		{
			name:    "success - GetBMCRecordingArchive anonymized",
			method:  "GetBMCRecordingArchiveAnonymized",
			nameArg: "one",
			idArg:   testBMCRecordingID,

			assertErr: require.NoError,
			want:      map[string]any{"body": "zip", "size": int64(3)},
			wantCalls: 2,
			wantRewrite: []*provisioning.BMCRecordingRewrite{
				provisioning.NewBMCRecordingRewrite(
					provisioning.Server{Name: "one"},
					api.BMCRecordingManifest{Server: "one", RecordingID: testBMCRecordingID},
				),
			},
		},
		{
			name:         "error - GetBMCRecordingArchive anonymized without manifest",
			method:       "GetBMCRecordingArchiveAnonymized",
			nameArg:      "one",
			idArg:        testBMCRecordingID,
			recordingErr: boom.Error,

			assertErr: boom.ErrorIs,
			wantCalls: 1,
		},
		{
			name:    "error - GetBMCRecordingArchive invalid ID",
			method:  "GetBMCRecordingArchive",
			nameArg: "one",
			idArg:   "..",

			assertErr: errassert.ValidationError,
		},
		{
			name:    "success - GetBMCRecordingFile",
			method:  "GetBMCRecordingFile",
			nameArg: "one",
			idArg:   testBMCRecordingID,

			assertErr: require.NoError,
			want:      map[string]any{"body": "manifest.json", "size": int64(13)},
			wantCalls: 1,
		},
		{
			name:    "success - DeleteBMCRecording",
			method:  "DeleteBMCRecording",
			nameArg: "one",
			idArg:   testBMCRecordingID,

			assertErr: require.NoError,
			wantCalls: 1,
		},
		{
			name:    "error - name empty",
			method:  "GetBMCRecordings",
			nameArg: "",

			assertErr: errassert.OperationNotPermittedError,
			want:      []api.BMCRecordingManifest(nil),
		},
		{
			name:             "error - unknown server",
			method:           "GetBMCRecordings",
			nameArg:          "one",
			repoGetByNameErr: domain.ErrNotFound,

			assertErr: errassert.NotFoundError,
			want:      []api.BMCRecordingManifest(nil),
		},
		{
			name:    "error - invalid ID",
			method:  "DeleteBMCRecording",
			nameArg: "one",
			idArg:   "../two",

			assertErr: errassert.ValidationError,
		},
		{
			name:         "error - repository",
			method:       "GetBMCRecordingArchive",
			nameArg:      "one",
			idArg:        testBMCRecordingID,
			recordingErr: boom.Error,

			assertErr:   boom.ErrorIs,
			wantCalls:   1,
			wantRewrite: []*provisioning.BMCRecordingRewrite{nil},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return &provisioning.Server{Name: name}, tc.repoGetByNameErr
				},
			}

			recordings := newBMCRecordingRepoMock(tc.recordingErr)

			svc := provisioningServer.New(repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithBMCRecordingRepo(recordings),
			)

			// Run test
			got, err := bmcRecordingMethods[tc.method](t.Context(), svc, tc.nameArg, tc.idArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)

			calls := len(recordings.GetAllCalls()) + len(recordings.GetByIDCalls()) + len(recordings.GetArchiveCalls()) + len(recordings.GetFileCalls()) + len(recordings.DeleteByIDCalls())
			require.Equal(t, tc.wantCalls, calls, "the call is only forwarded after the checks")

			var rewrites []*provisioning.BMCRecordingRewrite
			for _, call := range recordings.GetArchiveCalls() {
				rewrites = append(rewrites, call.Rewrite)
			}

			require.Equal(t, tc.wantRewrite, rewrites, "only an anonymized archive is rewritten, from the server and the manifest")
		})
	}
}

func TestServerService_BMCRecordingNotConfigured(t *testing.T) {
	t.Parallel()

	for method, call := range bmcRecordingMethods {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			// Setup
			svc := provisioningServer.New(&repoMock.ServerRepoMock{}, nil, nil, nil, nil, nil, nil, tls.Certificate{})

			// Run test
			_, err := call(t.Context(), svc, "one", testBMCRecordingID)

			// Assert
			errassert.OperationNotPermittedError(t, err, "recording is not available without repository")
		})
	}
}
