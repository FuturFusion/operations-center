package entities

import (
	"context"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

// Code generation directives.
//
//generate-database:mapper target update.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e update objects
//generate-database:mapper stmt -e update objects-by-UUID
//generate-database:mapper stmt -e update objects-by-Origin
//generate-database:mapper stmt -e update objects-by-Origin-and-Status
//generate-database:mapper stmt -e update objects-by-Status
//generate-database:mapper stmt -e update names
//generate-database:mapper stmt -e update names-by-Origin
//generate-database:mapper stmt -e update names-by-Origin-and-Status
//generate-database:mapper stmt -e update names-by-Status
//generate-database:mapper stmt -e update id
//generate-database:mapper stmt -e update create
//generate-database:mapper stmt -e update update
//generate-database:mapper stmt -e update delete-by-UUID
//
//generate-database:mapper method -e update ID
//generate-database:mapper method -e update Exists
//generate-database:mapper method -e update GetOne
//generate-database:mapper method -e update GetMany
//generate-database:mapper method -e update GetNames
//generate-database:mapper method -e update Create
//generate-database:mapper method -e update Update
//generate-database:mapper method -e update DeleteOne-by-UUID

func GetUpdatesByAssignedChannelName(ctx context.Context, tx dbtx, name string, filters ...provisioning.UpdateFilter) (provisioning.Updates, error) {
	stmt := fmt.Sprintf(`SELECT %s
  FROM channels
  JOIN channels_updates ON channels.id = channels_updates.channel_id
  JOIN updates ON channels_updates.update_id = updates.id
  WHERE channels.name = ?
`, updateColumns())

	args := []any{name}

	if len(filters) > 0 {
		filter := filters[0]

		if filter.UUID != nil {
			stmt += ` AND updates.uuid = ?`
			args = append(args, filter.UUID)
		}

		if filter.Origin != nil {
			stmt += ` AND updates.origin = ?`
			args = append(args, filter.Origin)
		}

		if filter.Status != nil {
			stmt += ` AND updates.status = ?`
			args = append(args, filter.Status)
		}
	}

	stmt += " ORDER BY updates.uuid"

	return getUpdatesRaw(ctx, tx, stmt, args...)
}
