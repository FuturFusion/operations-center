package dbschema

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lxc/incus/v7/shared/logger"

	"github.com/FuturFusion/operations-center/internal/sql/sqlite/query"
)

// doesSchemaTableExist return whether the schema table is present in the
// database.
func doesSchemaTableExist(ctx context.Context, tx *sql.Tx) (bool, error) {
	statement := `
SELECT COUNT(name) FROM sqlite_master WHERE type = 'table' AND name = 'schema'
`
	rows, err := tx.QueryContext(ctx, statement)
	if err != nil {
		return false, err
	}

	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return false, fmt.Errorf("schema table query returned no rows")
	}

	if rows.Err() != nil {
		return false, rows.Err()
	}

	if rows.Err() != nil {
		return false, rows.Err()
	}

	var count int

	err = rows.Scan(&count)
	if err != nil {
		return false, err
	}

	return count == 1, nil
}

// Return all versions in the schema table, in increasing order.
func selectSchemaVersions(ctx context.Context, tx *sql.Tx) ([]int, error) {
	stmt := `
SELECT version FROM schema ORDER BY version
`

	values := []int{}
	rows, err := tx.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var value int
		err := rows.Scan(&value)
		if err != nil {
			return nil, err
		}

		values = append(values, value)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return values, nil
}

// Create the schema table.
func createSchemaTable(tx *sql.Tx) error {
	statement := `
CREATE TABLE schema (
    id         INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    version    INTEGER NOT NULL,
    updated_at DATETIME NOT NULL,
    UNIQUE (version)
)
`
	_, err := tx.Exec(statement)
	return err
}

// Insert a new version into the schema table.
func insertSchemaVersion(tx *sql.Tx, newVersion int) error {
	statement := `
INSERT INTO schema (version, updated_at) VALUES (?, strftime("%s"))
`
	_, err := tx.Exec(statement, newVersion)
	return err
}

// Return the number of rows in the cluster_members table that have their address column
// set to '0.0.0.0'.
func selectUnclusteredNodesCount(ctx context.Context, tx *sql.Tx) (int, error) {
	return query.Count(ctx, tx, "cluster_members", "address='0.0.0.0'")
}

// Update the schema and api_extensions columns of the row in the cluster_members table
// that matches the given id.
//
// If not such row is found, an error is returned.
func updateNodeVersion(tx *sql.Tx, address string, apiExtensions int) error {
	stmt := "UPDATE cluster_members SET schema=?, api_extensions=? WHERE address=?"
	result, err := tx.Exec(stmt, len(updates), apiExtensions, address)
	if err != nil {
		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if n != 1 {
		return fmt.Errorf("updated %d rows instead of 1", n)
	}

	return nil
}

// Return a slice of binary integer tuples. Each tuple contains the schema
// version and number of api extensions of a node in the cluster.
func selectNodesVersions(ctx context.Context, tx *sql.Tx) ([][2]int, error) {
	versions := [][2]int{}
	stmt, err := tx.Prepare("SELECT schema, api_extensions FROM cluster_members WHERE state=0")
	if err != nil {
		// In order to make cluster updates work, let's check for "pending" as well as that's the column's previous name.
		stmt, err = tx.Prepare("SELECT schema, api_extensions FROM cluster_members WHERE pending=0")
		if err != nil {
			return nil, err
		}
	}
	defer logger.WarnOnError(stmt.Close, "Failed to close statement")
	err = query.SelectObjects(ctx, stmt, func(scan func(dest ...any) error) error {
		version := [2]int{}
		err := scan(&version[0], &version[1])
		if err != nil {
			return err
		}

		versions = append(versions, version)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return versions, nil
}

var errSomeNodesAreBehind = errors.New("Some cluster members are behind this cluster member's version")

func checkClusterIsUpgradable(ctx context.Context, tx *sql.Tx, target [2]int) error {
	// Get the current versions in the cluster_members table.
	versions, err := selectNodesVersions(ctx, tx)
	if err != nil {
		return fmt.Errorf("failed to fetch current cluster members versions: %w", err)
	}

	for _, ver := range versions {
		// Compare schema versions only.
		n, err := CompareVersions(target, ver, false)
		if err != nil {
			return err
		}

		switch n {
		case 0:
			// Versions are equal, there's hope for the
			// update. Let's check the next node.
			continue

		case 1:
			// Our version is bigger, we should stop here
			// and wait for other cluster_members to be upgraded and
			// restarted.
			return errSomeNodesAreBehind

		case 2:
			// Another node has a version greater than ours
			// and presumably is waiting for other cluster_members
			// to upgrade. Let's error out and shutdown
			// since we need a greater version.
			return errors.New("This cluster member's version is behind, please upgrade")

		default:
			panic("Unexpected return value from compareVersions")
		}
	}

	return nil
}
