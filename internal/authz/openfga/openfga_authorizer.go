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

	"github.com/FuturFusion/operations-center/internal/authz"
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

	f.client, err = client.NewSdkClient(&conf)
	if err != nil {
		return nil, fmt.Errorf("Failed to create OpenFGA client: %w", err)
	}

	err = f.ensureAuthorizationModel(ctx)
	if err != nil {
		return nil, err
	}

	return f, nil
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
