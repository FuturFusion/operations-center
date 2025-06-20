// Code generated by generate-inventory; DO NOT EDIT.

package incus

import (
	"context"
	"net/http"

	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
)

func (s serverClient) GetNetworks(ctx context.Context, connectionURL string) ([]incusapi.Network, error) {
	client, err := s.getClient(ctx, connectionURL)
	if err != nil {
		return nil, err
	}

	serverNetworks, err := client.GetNetworksAllProjects()
	if err != nil {
		return nil, err
	}

	return serverNetworks, nil
}

func (s serverClient) GetNetworkByName(ctx context.Context, connectionURL string, networkName string) (incusapi.Network, error) {
	client, err := s.getClient(ctx, connectionURL)
	if err != nil {
		return incusapi.Network{}, err
	}

	serverNetwork, _, err := client.GetNetwork(networkName)
	if incusapi.StatusErrorCheck(err, http.StatusNotFound) {
		return incusapi.Network{}, domain.ErrNotFound
	}

	if err != nil {
		return incusapi.Network{}, err
	}

	return *serverNetwork, nil
}
