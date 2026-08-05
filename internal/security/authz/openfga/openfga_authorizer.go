package openfga

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	"github.com/openfga/go-sdk/credentials"

	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

// FGA represents an OpenFGA authorizer.
type FGA struct {
	client *client.OpenFgaClient
}

var _ authz.Authorizer = FGA{}

func New(ctx context.Context, apiURL string, apiToken string, storeID string) (*FGA, error) {
	var err error
	f := &FGA{}

	f.client, err = newClient(apiURL, apiToken, storeID)
	if err != nil {
		return nil, err
	}

	err = f.ensureAuthorizationModel(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Failed to initialize OpenFGA authorization model, retrying in background", logger.Err(err))
		go f.retryEnsureAuthorizationModel(ctx)
	}

	return f, nil
}

func newClient(apiURL string, apiToken string, storeID string) (*client.OpenFgaClient, error) {
	conf := client.ClientConfiguration{
		ApiUrl:  apiURL,
		StoreId: storeID,
		Credentials: &credentials.Credentials{
			Method: credentials.CredentialsMethodApiToken,
			Config: &credentials.Config{
				ApiToken: apiToken,
			},
		},
	}

	c, err := client.NewSdkClient(&conf)
	if err != nil {
		return nil, fmt.Errorf("Failed to create OpenFGA client: %w", err)
	}

	return c, nil
}

// CheckConnectivity verifies OpenFGA is reachable with the given credentials
// and store.
func CheckConnectivity(ctx context.Context, apiURL string, apiToken string, storeID string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	c, err := newClient(apiURL, apiToken, storeID)
	if err != nil {
		return err
	}

	_, err = c.ReadLatestAuthorizationModel(ctx).Execute()
	if err != nil {
		return fmt.Errorf("Failed to connect to OpenFGA: %w", err)
	}

	return nil
}

// retryEnsureAuthorizationModel retries to ensure the authorization model
// until it succeeds or ctx is cancelled, e.g. if OpenFGA was not reachable
// when the authorizer was created.
func (f *FGA) retryEnsureAuthorizationModel(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
		}

		err := f.ensureAuthorizationModel(ctx)
		if err != nil {
			slog.WarnContext(ctx, "Failed to initialize OpenFGA authorization model, retrying", logger.Err(err))
			continue
		}

		slog.InfoContext(ctx, "OpenFGA authorization model initialized")

		return
	}
}

func (f FGA) ensureAuthorizationModel(ctx context.Context) error {
	// Load current authorization model.
	readModelResponse, err := f.client.ReadLatestAuthorizationModel(ctx).Execute()
	if err != nil {
		return fmt.Errorf("Failed to read pre-existing OpenFGA model: %w", err)
	}

	// Check if we need to upload an initial model.
	if readModelResponse.AuthorizationModel == nil {
		slog.InfoContext(ctx, "Upload initial OpenFGA model")

		// Upload the model itself.
		var builtinAuthorizationModel client.ClientWriteAuthorizationModelRequest
		err := json.Unmarshal([]byte(authModel), &builtinAuthorizationModel)
		if err != nil {
			return fmt.Errorf("Failed to unmarshal built in authorization model: %w", err)
		}

		_, err = f.client.WriteAuthorizationModel(ctx).Body(builtinAuthorizationModel).Execute()
		if err != nil {
			return fmt.Errorf("Failed to write the authorization model: %w", err)
		}

		// Allow basic authenticated access.
		err = f.sendTuples(ctx, []client.ClientTupleKey{
			{User: "user:*", Relation: "authenticated", Object: authz.ObjectServer().String()},
		}, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func (f FGA) CheckPermission(ctx context.Context, details *authz.RequestDetails, object authz.Object, entitlement authz.Entitlement) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	username := details.Username

	objectUser := authz.ObjectUser(username)
	body := client.ClientCheckRequest{
		User:     objectUser.String(),
		Relation: string(entitlement),
		Object:   object.String(),
	}

	slog.DebugContext(ctx, "Checking OpenFGA relation", slog.Any("object", object), slog.Any("entitlement", entitlement), slog.String("url", details.URL.String()), slog.String("method", details.Method), slog.String("username", username), slog.String("protocol", details.Protocol))
	resp, err := f.client.Check(ctx).Body(body).Execute()
	if err != nil {
		return fmt.Errorf("Failed to check OpenFGA relation: %w", err)
	}

	if !resp.GetAllowed() {
		return api.StatusErrorf(http.StatusForbidden, "User does not have entitlement %q on object %q", entitlement, object)
	}

	return nil
}

// sendTuples directly sends the write/deletion tuples to OpenFGA.
func (f *FGA) sendTuples(ctx context.Context, writes []client.ClientTupleKey, deletions []client.ClientTupleKeyWithoutCondition) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	opts := client.ClientWriteOptions{
		Transaction: &client.TransactionOptions{
			Disable:             true,
			MaxParallelRequests: 5,
			MaxPerChunk:         50,
		},
	}

	body := client.ClientWriteRequest{
		Writes:  []client.ClientTupleKey{},
		Deletes: []openfga.TupleKeyWithoutCondition{},
	}

	if writes != nil {
		body.Writes = writes
	}

	if deletions != nil {
		body.Deletes = deletions
	}

	clientWriteResponse, err := f.client.Write(ctx).Options(opts).Body(body).Execute()
	if err != nil {
		return fmt.Errorf("Failed to write to OpenFGA store: %w", err)
	}

	errs := []error{}

	for _, write := range clientWriteResponse.Writes {
		if write.Error != nil {
			errs = append(errs, fmt.Errorf("Failed to write tuple to OpenFGA store (user: %q; relation: %q; object: %q): %w", write.TupleKey.User, write.TupleKey.Relation, write.TupleKey.Object, write.Error))
		}
	}

	for _, deletion := range clientWriteResponse.Deletes {
		if deletion.Error != nil {
			errs = append(errs, fmt.Errorf("Failed to delete tuple from OpenFGA store (user: %q; relation: %q; object: %q): %w", deletion.TupleKey.User, deletion.TupleKey.Relation, deletion.TupleKey.Object, deletion.Error))
		}
	}

	return errors.Join(errs...)
}
