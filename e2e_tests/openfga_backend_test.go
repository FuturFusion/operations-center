package e2e

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	openfgasdk "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/openfga"
)

const (
	openFGAImage     = "openfga/openfga:v1.8.9"
	openFGAStoreName = "operations-center-e2e"

	// Operations Center requires an API token. OpenFGA runs without
	// authentication and ignores it.
	openFGAAPIToken = "dummy"

	openFGAObject = "server:operations-center"
)

// openFGABackend is an OpenFGA server. It runs as a container on the end 2 end
// test host.
type openFGABackend struct {
	// APIURL is the URL Operations Center uses to reach OpenFGA.
	APIURL  string
	StoreID string

	client *client.OpenFgaClient
}

func startOpenFGA(t *testing.T) *openFGABackend {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	backendHostAddress := e2eHostAddress(t)

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(5*time.Minute))
	defer cancel()

	container, err := openfga.Run(ctx, openFGAImage)

	// Register the cleanup before the error check. A failed start can leave a
	// container behind.
	t.Cleanup(func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		err := testcontainers.TerminateContainer(container)
		if err != nil {
			t.Errorf("Failed to terminate the OpenFGA container: %v", err)
		}
	})

	require.NoError(t, err, "Failed to start the OpenFGA container")

	localURL, err := container.HttpEndpoint(ctx)
	require.NoError(t, err, "Failed to get the endpoint of the OpenFGA container")

	port, err := container.MappedPort(ctx, "8080/tcp")
	require.NoError(t, err, "Failed to get the port of the OpenFGA container")

	fgaClient, err := client.NewSdkClient(&client.ClientConfiguration{ApiUrl: localURL})
	require.NoError(t, err, "Failed to create the OpenFGA client")

	store, err := fgaClient.CreateStore(ctx).Body(client.ClientCreateStoreRequest{Name: openFGAStoreName}).Execute()
	require.NoError(t, err, "Failed to create the OpenFGA store")

	err = fgaClient.SetStoreId(store.GetId())
	require.NoError(t, err, "Failed to select the OpenFGA store")

	backend := &openFGABackend{
		APIURL:  fmt.Sprintf("http://%s", net.JoinHostPort(backendHostAddress, port.Port())),
		StoreID: store.GetId(),
		client:  fgaClient,
	}

	t.Logf("OpenFGA is listening on %s, store %s", backend.APIURL, backend.StoreID)

	return backend
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
