package dbschema

import (
	"context"
	"database/sql"
)

const freshSchema = `
CREATE TABLE tokens (
  uuid TEXT PRIMARY KEY NOT NULL,
  uses_remaining INTEGER NOT NULL,
  expire_at TEXT NOT NULL,
  description TEXT NOT NULL
);

CREATE TABLE clusters (
  id INTEGER PRIMARY KEY NOT NULL,
  name TEXT NOT NULL,
  server_hostnames TEXT NOT NULL,
  UNIQUE (name)
);

INSERT INTO schema (version, updated_at) VALUES (2, strftime("%s"))
`

/* Database updates are one-time actions that are needed to move an
   existing database from one version of the schema to the next.

   Those updates are applied at startup time before anything else
   is initialized. This means that they should be entirely
   self-contained and not touch anything but the database.

   Calling API functions isn't allowed as such functions may themselves
   depend on a newer DB schema and so would fail when upgrading a very old
   version.

   DO NOT USE this mechanism for one-time actions which do not involve
   changes to the database schema.

   Only append to the updates list, never remove entries and never re-order them.
*/

var updates = map[int]update{
	1: updateFromV0,
	2: updateFromV1,
}

func updateFromV0(ctx context.Context, tx *sql.Tx) error {
	// v0..v1 the dawn of operations center
	stmt := ``
	_, err := tx.Exec(stmt)
	return mapDBError(err)
}

func updateFromV1(ctx context.Context, tx *sql.Tx) error {
	// v1..v2 add tokens table
	stmt := `
CREATE TABLE tokens (
  uuid TEXT PRIMARY KEY NOT NULL,
  uses_remaining INTEGER NOT NULL,
  expire_at TEXT NOT NULL,
  description TEXT NOT NULL
);

CREATE TABLE clusters (
  id INTEGER PRIMARY KEY NOT NULL,
  name TEXT NOT NULL,
  server_hostnames TEXT NOT NULL,
  UNIQUE (name)
);
`
	_, err := tx.Exec(stmt)
	return mapDBError(err)
}
