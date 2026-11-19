package entities

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Code generation directives.
//
//generate-database:mapper target config.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e config_set objects table=config
//generate-database:mapper stmt -e config_set objects-by-NodeID table=config
//generate-database:mapper stmt -e config_set id table=config
//generate-database:mapper stmt -e config_set create table=config
//generate-database:mapper stmt -e config_set update table=config
//generate-database:mapper stmt -e config_set delete-by-NodeID table=config
//
//generate-database:mapper method -e config_set GetMany table=config
//generate-database:mapper method -e config_set GetOne table=config
//generate-database:mapper method -e config_set ID table=config
//generate-database:mapper method -e config_set Create table=config
//generate-database:mapper method -e config_set Update table=config
//generate-database:mapper method -e config_set DeleteOne-by-NodeID table=config

type ConfigSet struct {
	ID     int64
	NodeID sql.NullInt64 `db:"primary=yes&nullable=true"`

	Data json.RawMessage `db:"marshal=json"`
}

type ConfigSetFilter struct {
	NodeID *sql.NullInt64
}

// configSetUpdateByNodeID matches a NULL node ID, unlike the generated statements selecting the ID.
var configSetUpdateByNodeID = RegisterStmt(`
UPDATE config
  SET data = ?
 WHERE (node_id = ? OR ( node_id IS NULL AND ? IS NULL))
`)

// UpdateConfigSetData updates the data of the config_set with the given node ID.
// Unlike UpdateConfigSet, it also updates the shared config_set without a node ID.
func UpdateConfigSetData(ctx context.Context, tx dbtx, nodeID sql.NullInt64, data json.RawMessage) (_err error) {
	defer func() {
		_err = mapErr(_err, "Config_set")
	}()

	stmt, err := StmtString(configSetUpdateByNodeID)
	if err != nil {
		return err
	}

	marshaledData, err := marshalJSON(data)
	if err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, stmt, marshaledData, nodeID, nodeID)
	if err != nil {
		return fmt.Errorf("Update \"config\" entry failed: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("Fetch affected rows: %w", err)
	}

	if n != 1 {
		return ErrNotFound
	}

	return nil
}
