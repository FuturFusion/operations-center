package entities

import (
	"encoding/json"

	incusapi "github.com/lxc/incus/v6/shared/api"
)

// Code generation directives.
//
//go:generate -command mapper go run github.com/FuturFusion/operations-center/cmd/generate-database db mapper -t storage_volumes.mapper.go
//go:generate mapper reset --no-incus-api
//
//go:generate mapper stmt -e storage_volume objects table=storage_volumes
//go:generate mapper stmt -e storage_volume objects-by-Name-and-ServerID-and-ProjectID table=storage_volumes
//go:generate mapper stmt -e storage_volume objects-by-ProjectID table=storage_volumes
//go:generate mapper stmt -e storage_volume id table=storage_volumes
//go:generate mapper stmt -e storage_volume create table=storage_volumes
//go:generate mapper stmt -e storage_volume delete-by-Name-and-ServerID-and-ProjectID table=storage_volumes
//
//go:generate mapper method -e storage_volume ID table=storage_volumes --map-error
//go:generate mapper method -e storage_volume Exists table=storage_volumes --map-error
//go:generate mapper method -e storage_volume GetOne table=storage_volumes --map-error
//go:generate mapper method -e storage_volume GetMany table=storage_volumes --map-error
//go:generate mapper method -e storage_volume Create table=storage_volumes --map-error
//go:generate mapper method -e storage_volume DeleteOne-by-Name-and-ServerID-and-ProjectID table=storage_volumes --map-error
//
//go:generate gofmt -s -w .
//go:generate go run golang.org/x/tools/cmd/goimports -w .

type StorageVolume struct {
	ID          int64
	ServerID    int64               `db:"primary=yes&join=servers.id"`
	ProjectID   int64               `db:"primary=yes"`
	Name        string              `db:"primary=yes"`
	Object      StorageVolumeObject `db:"marshal=yes"`
	LastUpdated string
}

type StorageVolumeFilter struct {
	ServerID  *int64
	ProjectID *int64
	Name      *string
}

type StorageVolumeObject incusapi.StorageVolume

// MarshalText implements the encoding.TextMarshaler interface.
func (s StorageVolumeObject) MarshalDB() ([]byte, error) {
	return json.Marshal(s)
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (s *StorageVolumeObject) UnmarshalDB(text string) error {
	var sv incusapi.StorageVolume
	err := json.Unmarshal([]byte(text), &sv)
	if err != nil {
		return err
	}

	*s = StorageVolumeObject(sv)

	return nil
}
