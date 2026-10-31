package dbschema

import (
	"context"
	"database/sql"
	"maps"
)

var freshSchemaLocal string

var updatesLocal = map[int]update{
	1: localUpdateFromV0,
}

func LocalUpdates() map[int]update {
	mapCopy := make(map[int]update, len(updatesLocal))
	maps.Copy(mapCopy, updatesLocal)

	return mapCopy
}

func LocalFreshSchema() string {
	return freshSchemaLocal
}

func localUpdateFromV0(ctx context.Context, tx *sql.Tx) error {
	stmt := `CREATE TABLE raft_nodes (
     id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
     address TEXT NOT NULL,
     role INTEGER NOT NULL DEFAULT 0,
     name TEXT NOT NULL default "",
     certificate TEXT,
     UNIQUE (address)
 );

CREATE TABLE patches (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    name VARCHAR(255) NOT NULL,
    applied_at DATETIME NOT NULL,
    UNIQUE (name)
);
 `

	_, err := tx.Exec(stmt)
	return MapDBError(err)
}
