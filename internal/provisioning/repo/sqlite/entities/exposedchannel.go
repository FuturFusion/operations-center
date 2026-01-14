package entities

import (
	"context"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

// Code generation directives.
//
//generate-database:mapper target exposedchannel.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e exposedchannel objects table=exposedchannels
//generate-database:mapper stmt -e exposedchannel objects-by-Name table=exposedchannels
//generate-database:mapper stmt -e exposedchannel names table=exposedchannels
//generate-database:mapper stmt -e exposedchannel id table=exposedchannels
//generate-database:mapper stmt -e exposedchannel create table=exposedchannels
//generate-database:mapper stmt -e exposedchannel update table=exposedchannels
//generate-database:mapper stmt -e exposedchannel rename table=exposedchannels
//generate-database:mapper stmt -e exposedchannel delete-by-Name table=exposedchannels
//
//generate-database:mapper method -e exposedchannel ID
//generate-database:mapper method -e exposedchannel Exists
//generate-database:mapper method -e exposedchannel GetOne
//generate-database:mapper method -e exposedchannel GetMany
//generate-database:mapper method -e exposedchannel GetNames
//generate-database:mapper method -e exposedchannel Create
//generate-database:mapper method -e exposedchannel Update
//generate-database:mapper method -e exposedchannel Rename
//generate-database:mapper method -e exposedchannel DeleteOne-by-Name
//

func GetUpdatesByAssignedExposedchannelName(ctx context.Context, tx dbtx, name string) (provisioning.Updates, error) {
	stmt := fmt.Sprintf(`SELECT %s
	FROM exposedchannels
	JOIN exposedchannels_updates ON exposedchannels.id = exposedchannels_updates.exposedchannel_id
	JOIN updates ON exposedchannels_updates.update_id = updates.id
	ORDER BY updates.uuid
	`, updateColumns())

	return getUpdatesRaw(ctx, tx, stmt)
}
