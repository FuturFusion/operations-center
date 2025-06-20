// Code generated by generate-inventory; DO NOT EDIT.

package incus

import (
	"context"
	"net/http"

	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
)

func (s serverClient) GetStorageVolumes(ctx context.Context, connectionURL string, storageVolumeName string) ([]incusapi.StorageVolume, error) {
	client, err := s.getClient(ctx, connectionURL)
	if err != nil {
		return nil, err
	}

	serverStorageVolumes, err := client.GetStoragePoolVolumesAllProjects(storageVolumeName)
	if err != nil {
		return nil, err
	}

	return serverStorageVolumes, nil
}

func (s serverClient) GetStorageVolumeByName(ctx context.Context, connectionURL string, storagePoolName string, storageVolumeName string, storageVolumeType string) (incusapi.StorageVolume, error) {
	client, err := s.getClient(ctx, connectionURL)
	if err != nil {
		return incusapi.StorageVolume{}, err
	}

	serverStorageVolume, _, err := client.GetStoragePoolVolume(storagePoolName, storageVolumeType, storageVolumeName)
	if incusapi.StatusErrorCheck(err, http.StatusNotFound) {
		return incusapi.StorageVolume{}, domain.ErrNotFound
	}

	if err != nil {
		return incusapi.StorageVolume{}, err
	}

	return *serverStorageVolume, nil
}
