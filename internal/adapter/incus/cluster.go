package incus

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"time"

	incus "github.com/lxc/incus/v7/client"
	incusapi "github.com/lxc/incus/v7/shared/api"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/logger"
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

	op, err := updateCluster(ctx, client, incusapi.ClusterPut{
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
	if errors.Is(err, errLostEventListener) {
		err = c.recoverClusterJoin(ctx, server, endpoint.GetCertificate(), op.ID, err)
	}

	if err != nil {
		return fmt.Errorf("Failed to update cluster during cluster join on %q (%s): %w", server.GetName(), server.GetConnectionURL(), err)
	}

	return nil
}

// recoverClusterJoin reconnects and waits for the join operation if the event listener
// got lost, using both the cluster certificate and the server certificate.
func (c Client) recoverClusterJoin(ctx context.Context, server provisioning.Server, clusterCertificate string, opID string, lostErr error) error {
	timeout := -1
	deadline, ok := ctx.Deadline()
	if ok {
		timeout = int(time.Until(deadline).Seconds())
	}

	endpoints := []struct {
		whichCert string
		endpoint  provisioning.Endpoint
	}{
		{
			whichCert: "the cluster",
			endpoint: provisioning.Server{
				Name:          server.Name,
				ConnectionURL: server.ConnectionURL,
				Certificate:   new(clusterCertificate),
			},
		},
		{
			whichCert: "its own",
			endpoint:  server,
		},
	}

	var err error
	for _, e := range endpoints {
		var op *incusapi.Operation
		op, err = c.getOperationWait(ctx, e.endpoint, opID, timeout)

		var certErr *tls.CertificateVerificationError
		if errors.As(err, &certErr) {
			continue // try the other certificate
		}

		if incusapi.StatusErrorCheck(err, http.StatusNotFound) {
			err = fmt.Errorf("Operation %q no longer exists, it likely finished over 5s ago (server presents %s certificate), check whether %q joined the cluster: %w", opID, e.whichCert, server.Name, err)
			break
		}

		if err != nil {
			break
		}

		if op.Err != "" {
			err = fmt.Errorf("Operation %q failed after recovery: %s", opID, op.Err)
			break
		}

		if !op.StatusCode.IsFinal() {
			err = fmt.Errorf("Operation %q was still running when the wait timed out", opID)
			break
		}

		if op.StatusCode != incusapi.Success {
			return fmt.Errorf("Operation %q ended with status %q", opID, op.Status)
		}

		slog.WarnContext(ctx, "Recovered cluster join after losing the event listener", slog.String("server", server.Name), slog.String("operation", opID), logger.Err(lostErr))

		return nil
	}

	return fmt.Errorf("%w, failed to recover: %w", lostErr, err)
}

func (c Client) getOperationWait(ctx context.Context, endpoint provisioning.Endpoint, opID string, timeout int) (*incusapi.Operation, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	op, _, err := client.GetOperationWait(opID, timeout)

	return op, err
}

var errLostEventListener = errors.New("Lost connection to the event listener")

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

	start := time.Now()
	op, err := client.UpdateCluster(req, "")
	if err != nil {
		return incusapi.Operation{}, err
	}

	opID := op.Get().ID

	for {
		select {
		case <-ctx.Done():
			return incusapi.Operation{}, fmt.Errorf("Stopped waiting %s into operation %q, its outcome is unknown: %w", time.Since(start).Round(time.Second), opID, ctx.Err())

		case event, ok := <-events:
			if !ok {
				err = listener.Wait()
				if err == nil {
					err = errors.New("Event listener was disconnected")
				}

				return op.Get(), fmt.Errorf("%w %s into operation %q, its outcome is unknown: %w", errLostEventListener, time.Since(start).Round(time.Second), opID, err)
			}

			opAPI := incusapi.Operation{}
			err = json.Unmarshal(event.Metadata, &opAPI)
			if err != nil {
				slog.WarnContext(ctx, "Failed to decode operation event", slog.String("operation", opID), logger.Err(err))
				continue
			}

			if opAPI.ID != opID || !opAPI.StatusCode.IsFinal() {
				continue
			}

			if opAPI.Err != "" {
				return incusapi.Operation{}, errors.New(opAPI.Err)
			}

			if opAPI.StatusCode != incusapi.Success {
				return incusapi.Operation{}, fmt.Errorf("Operation %q ended with status %q", opID, opAPI.Status)
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
