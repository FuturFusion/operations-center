package incus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"

	incus "github.com/lxc/incus/v7/client"
	incusapi "github.com/lxc/incus/v7/shared/api"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

func (c Client) SetServerConfig(ctx context.Context, endpoint provisioning.Endpoint, config map[string]string) error {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return err
	}

	svr, etag, err := client.GetServer()
	if err != nil {
		return fmt.Errorf("Failed to get current config from %q (%s): %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	if svr.Config == nil {
		svr.Config = map[string]string{}
	}

	maps.Copy(svr.Config, config)

	err = client.UpdateServer(svr.Writable(), etag)
	if err != nil {
		return fmt.Errorf("Failed to set config on %q (%s): %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	return nil
}

func (c Client) EnableCluster(ctx context.Context, server provisioning.Server) (clusterCertificate string, _ error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return "", err
	}

	req := incusapi.ClusterPut{
		Cluster: incusapi.Cluster{
			ServerName: server.Name,
			Enabled:    true,
		},
	}

	op, err := updateCluster(ctx, client, req)
	if err != nil {
		return "", fmt.Errorf("Failed to update cluster on %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	anyClusterCertificate, ok := op.Metadata["certificate"]
	if !ok {
		return "", nil
	}

	clusterCertificate, ok = anyClusterCertificate.(string)
	if !ok {
		return "", nil
	}

	return clusterCertificate, nil
}

func (c Client) GetClusterNodeNames(ctx context.Context, endpoint provisioning.Endpoint) ([]string, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	nodeNames, err := client.GetClusterMemberNames()
	if err != nil {
		return nil, fmt.Errorf("Failed to get cluster node names on %q (%s): %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	return nodeNames, nil
}

func (c Client) GetClusterJoinToken(ctx context.Context, endpoint provisioning.Endpoint, memberName string) (joinToken string, _ error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return "", err
	}

	op, err := client.CreateClusterMember(incusapi.ClusterMembersPost{
		ServerName: memberName,
	})
	if err != nil {
		return "", fmt.Errorf("Failed to get cluster join token on %q (%s): %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	opAPI := op.Get()
	token, err := opAPI.ToClusterJoinToken()
	if err != nil {
		return "", fmt.Errorf("Failed converting token operation to join token: %w", err)
	}

	return token.String(), nil
}

func (c Client) JoinCluster(ctx context.Context, server provisioning.Server, joinToken string, serverAddressOfClusterRole string, endpoint provisioning.Endpoint, config []api.ClusterMemberConfigKey) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	// Ignore error, connection URL has been parsed by incus client already.
	clusterAddressURL, _ := url.Parse(endpoint.GetConnectionURL())

	_, err = updateCluster(ctx, client, incusapi.ClusterPut{
		Cluster: incusapi.Cluster{
			ServerName:   server.Name,
			Enabled:      true,
			MemberConfig: config,
		},
		ClusterCertificate: endpoint.GetCertificate(),
		ServerAddress:      serverAddressOfClusterRole,
		ClusterToken:       joinToken,
		ClusterAddress:     clusterAddressURL.Host,
	})
	if err != nil {
		return fmt.Errorf("Failed to update cluster during cluster join on %q (%s): %w", server.GetName(), server.GetConnectionURL(), err)
	}

	return nil
}

// updateCluster updates the cluster configuration and waits for the operation.
//
// The server switches to the cluster certificate while the operation runs.
// New connections then fail the verification against the pinned server
// certificate. Therefore follow the operation on the events websocket, which
// is opened before the update.
func updateCluster(ctx context.Context, client incus.InstanceServer, req incusapi.ClusterPut) (incusapi.Operation, error) {
	// Closes the events websocket, including the listener of the operation.
	defer client.Disconnect()

	listener, err := client.GetEvents()
	if err != nil {
		return incusapi.Operation{}, fmt.Errorf("Failed to connect to the event listener: %w", err)
	}

	events := listener.AddChannel([]string{incusapi.EventTypeOperation}, 0)

	op, err := client.UpdateCluster(req, "")
	if err != nil {
		return incusapi.Operation{}, err
	}

	opID := op.Get().ID

	for {
		select {
		case <-ctx.Done():
			return incusapi.Operation{}, ctx.Err()

		case event, ok := <-events:
			if !ok {
				err = listener.Wait()
				if err == nil {
					err = errors.New("Event listener was disconnected")
				}

				return incusapi.Operation{}, fmt.Errorf("Lost connection to the event listener while waiting for operation %q: %w", opID, err)
			}

			opAPI := incusapi.Operation{}
			err = json.Unmarshal(event.Metadata, &opAPI)
			if err != nil || opAPI.ID != opID || !opAPI.StatusCode.IsFinal() {
				continue
			}

			if opAPI.Err != "" {
				return incusapi.Operation{}, errors.New(opAPI.Err)
			}

			return opAPI, nil
		}
	}
}

func (c Client) UpdateClusterCertificate(ctx context.Context, endpoint provisioning.Endpoint, certificatePEM string, keyPEM string) error {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return err
	}

	return client.UpdateClusterCertificate(incusapi.ClusterCertificatePut{
		ClusterCertificate:    certificatePEM,
		ClusterCertificateKey: keyPEM,
	}, "")
}
