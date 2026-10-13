package e2e

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	openfgasdk "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testingnet "github.com/FuturFusion/operations-center/internal/util/testing/net"
)

const (
	openFGAStoreName = "operations-center-e2e"

	// Operations Center requires an API token. OpenFGA runs without
	// authentication and ignores it.
	openFGAAPIToken = "dummy"

	openFGAObject = "server:operations-center"
)

// openFGABackend is an OpenFGA server. It runs as a process on the end 2 end
// test host.
type openFGABackend struct {
	// APIURL is the URL Operations Center uses to reach OpenFGA.
	APIURL  string
	StoreID string

	client *client.OpenFgaClient
}

func startOpenFGA(t *testing.T, tmpDir string) *openFGABackend {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	backendHostAddress := e2eHostAddress(t)
	binary := ensureOpenFGABinary(t, tmpDir)

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(time.Minute))
	defer cancel()

	httpPort := testingnet.FreePort(t, "0.0.0.0")
	grpcPort := testingnet.FreePort(t, "127.0.0.1")

	logFile, err := os.Create(filepath.Join(tmpDir, "openfga.log"))
	require.NoError(t, err, "Failed to create the OpenFGA log file")

	// Listen on all the interfaces. The Operations Center VM reaches OpenFGA
	// at an address of the host that is not known here.
	// Do not use ctx for the command. OpenFGA has to outlive this function.
	cmd := exec.Command(binary, "run", //nolint:noctx
		"--datastore-engine", "memory",
		"--http-addr", net.JoinHostPort("0.0.0.0", httpPort),
		"--grpc-addr", net.JoinHostPort("127.0.0.1", grpcPort),
		"--playground-enabled=false",
		"--metrics-enabled=false",
		"--log-format", "json",
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	err = cmd.Start()
	closeErr := logFile.Close()
	require.NoError(t, err, "Failed to start OpenFGA")

	// The goroutine sets exitErr before it closes exited.
	var exitErr error

	exited := make(chan struct{})
	go func() {
		exitErr = cmd.Wait()

		close(exited)
	}()

	ready := false

	t.Cleanup(func() {
		select {
		case <-exited:
			// A failed start is already reported.
			if ready {
				t.Errorf("OpenFGA exited before the end of the test: %v, see %s", exitErr, logFile.Name())
			}

			return

		default:
		}

		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// Signal fails if OpenFGA already exited. This is not an error.
		_ = cmd.Process.Signal(syscall.SIGTERM)

		select {
		case <-exited:
		case <-time.After(strechedTimeout(30 * time.Second)):
			t.Errorf("Failed to stop OpenFGA, kill it: %v", cmd.Process.Kill())
		}
	})

	require.NoError(t, closeErr, "Failed to close the OpenFGA log file")

	localURL := "http://" + net.JoinHostPort("127.0.0.1", httpPort)

	err = waitForOpenFGA(ctx, localURL, exited, &exitErr)
	require.NoErrorf(t, err, "OpenFGA did not get ready, see %s", logFile.Name())

	ready = true

	fgaClient, err := client.NewSdkClient(&client.ClientConfiguration{ApiUrl: localURL})
	require.NoError(t, err, "Failed to create the OpenFGA client")

	store, err := fgaClient.CreateStore(ctx).Body(client.ClientCreateStoreRequest{Name: openFGAStoreName}).Execute()
	require.NoError(t, err, "Failed to create the OpenFGA store")

	err = fgaClient.SetStoreId(store.GetId())
	require.NoError(t, err, "Failed to select the OpenFGA store")

	backend := &openFGABackend{
		APIURL:  "http://" + net.JoinHostPort(backendHostAddress, httpPort),
		StoreID: store.GetId(),
		client:  fgaClient,
	}

	t.Logf("OpenFGA is listening on %s, store %s", backend.APIURL, backend.StoreID)

	return backend
}

// waitForOpenFGA waits until the health check of OpenFGA succeeds. It returns
// an error, if OpenFGA exits or if ctx is done. exitErr is valid after exited
// is closed.
func waitForOpenFGA(ctx context.Context, url string, exited <-chan struct{}, exitErr *error) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/healthz", nil)
		if err != nil {
			return err
		}

		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				return nil
			}

			err = fmt.Errorf("unexpected status %q of the health check", resp.Status)
		}

		select {
		case <-exited:
			return fmt.Errorf("OpenFGA exited: %w", errors.Join(*exitErr, err))

		case <-ctx.Done():
			return errors.Join(ctx.Err(), err)

		case <-ticker.C:
		}
	}
}

// mustSetRole makes role the only role of user. An empty role removes all the
// roles of user.
func (b *openFGABackend) mustSetRole(t *testing.T, user string, role string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(30*time.Second))
	defer cancel()

	fgaUser := "user:" + user

	body := client.ClientWriteRequest{}

	tuples, err := b.readTuples(ctx, fgaUser)
	require.NoErrorf(t, err, "Failed to read the tuples of user %q from OpenFGA", user)

	// OpenFGA rejects the deletion of a missing tuple. Delete only the existing
	// tuples.
	for _, tuple := range tuples {
		body.Deletes = append(body.Deletes, client.ClientTupleKeyWithoutCondition{
			User:     tuple.User,
			Relation: tuple.Relation,
			Object:   tuple.Object,
		})
	}

	if role != "" {
		body.Writes = append(body.Writes, client.ClientTupleKey{
			User:     fgaUser,
			Relation: role,
			Object:   openFGAObject,
		})
	}

	if len(body.Deletes) == 0 && len(body.Writes) == 0 {
		return
	}

	_, err = b.client.Write(ctx).Body(body).Execute()
	require.NoErrorf(t, err, "Failed to set the role %q of user %q in OpenFGA", role, user)
}

func (b *openFGABackend) readTuples(ctx context.Context, fgaUser string) ([]openfgasdk.TupleKey, error) {
	object := openFGAObject

	resp, err := b.client.Read(ctx).Body(client.ClientReadRequest{User: &fgaUser, Object: &object}).Execute()
	if err != nil {
		return nil, err
	}

	tuples := make([]openfgasdk.TupleKey, 0, len(resp.GetTuples()))
	for _, tuple := range resp.GetTuples() {
		tuples = append(tuples, tuple.GetKey())
	}

	return tuples, nil
}

// mustAssertInitialized verifies that Operations Center uploaded its
// authorization model and the tuple for basic authenticated access.
func (b *openFGABackend) mustAssertInitialized(t *testing.T) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(30*time.Second))
	defer cancel()

	// Operations Center retries a failed upload of the authorization model in
	// the background.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		model, err := b.client.ReadLatestAuthorizationModel(ctx).Execute()
		require.NoError(c, err, "Failed to read the authorization model from OpenFGA")
		require.NotNil(c, model.AuthorizationModel, "expect Operations Center to upload its authorization model")

		tuples, err := b.readTuples(ctx, "user:*")
		require.NoError(c, err, "Failed to read the tuples of all the users from OpenFGA")
		require.Len(c, tuples, 1, "expect exactly one tuple for all the users")
		require.Equal(c, "authenticated", tuples[0].Relation, "expect Operations Center to grant basic authenticated access")
	}, strechedTimeout(30*time.Second), 500*time.Millisecond)
}
