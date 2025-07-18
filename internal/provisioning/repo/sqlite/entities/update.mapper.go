// Code generated by generate-database from the incus project - DO NOT EDIT.

package entities

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"
)

var updateObjects = RegisterStmt(`
SELECT updates.id, updates.uuid, updates.origin, updates.external_id, updates.version, updates.published_at, updates.severity, updates.channel, updates.changelog, updates.files, updates.url
  FROM updates
  ORDER BY updates.uuid
`)

var updateObjectsByUUID = RegisterStmt(`
SELECT updates.id, updates.uuid, updates.origin, updates.external_id, updates.version, updates.published_at, updates.severity, updates.channel, updates.changelog, updates.files, updates.url
  FROM updates
  WHERE ( updates.uuid = ? )
  ORDER BY updates.uuid
`)

var updateObjectsByChannel = RegisterStmt(`
SELECT updates.id, updates.uuid, updates.origin, updates.external_id, updates.version, updates.published_at, updates.severity, updates.channel, updates.changelog, updates.files, updates.url
  FROM updates
  WHERE ( updates.channel = ? )
  ORDER BY updates.uuid
`)

var updateObjectsByOrigin = RegisterStmt(`
SELECT updates.id, updates.uuid, updates.origin, updates.external_id, updates.version, updates.published_at, updates.severity, updates.channel, updates.changelog, updates.files, updates.url
  FROM updates
  WHERE ( updates.origin = ? )
  ORDER BY updates.uuid
`)

var updateNames = RegisterStmt(`
SELECT updates.uuid
  FROM updates
  ORDER BY updates.uuid
`)

var updateNamesByChannel = RegisterStmt(`
SELECT updates.uuid
  FROM updates
  WHERE ( updates.channel = ? )
  ORDER BY updates.uuid
`)

var updateID = RegisterStmt(`
SELECT updates.id FROM updates
  WHERE updates.uuid = ?
`)

var updateCreate = RegisterStmt(`
INSERT INTO updates (uuid, origin, external_id, version, published_at, severity, channel, changelog, files, url)
  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`)

var updateUpdate = RegisterStmt(`
UPDATE updates
  SET uuid = ?, origin = ?, external_id = ?, version = ?, published_at = ?, severity = ?, channel = ?, changelog = ?, files = ?, url = ?
 WHERE id = ?
`)

var updateDeleteByUUID = RegisterStmt(`
DELETE FROM updates WHERE uuid = ?
`)

// GetUpdateID return the ID of the update with the given key.
// generator: update ID
func GetUpdateID(ctx context.Context, db tx, uuid uuid.UUID) (_ int64, _err error) {
	defer func() {
		_err = mapErr(_err, "Update")
	}()

	stmt, err := Stmt(db, updateID)
	if err != nil {
		return -1, fmt.Errorf("Failed to get \"updateID\" prepared statement: %w", err)
	}

	row := stmt.QueryRowContext(ctx, uuid)
	var id int64
	err = row.Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return -1, ErrNotFound
	}

	if err != nil {
		return -1, fmt.Errorf("Failed to get \"updates\" ID: %w", err)
	}

	return id, nil
}

// UpdateExists checks if a update with the given key exists.
// generator: update Exists
func UpdateExists(ctx context.Context, db dbtx, uuid uuid.UUID) (_ bool, _err error) {
	defer func() {
		_err = mapErr(_err, "Update")
	}()

	stmt, err := Stmt(db, updateID)
	if err != nil {
		return false, fmt.Errorf("Failed to get \"updateID\" prepared statement: %w", err)
	}

	row := stmt.QueryRowContext(ctx, uuid)
	var id int64
	err = row.Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("Failed to get \"updates\" ID: %w", err)
	}

	return true, nil
}

// GetUpdate returns the update with the given key.
// generator: update GetOne
func GetUpdate(ctx context.Context, db dbtx, uuid uuid.UUID) (_ *provisioning.Update, _err error) {
	defer func() {
		_err = mapErr(_err, "Update")
	}()

	filter := provisioning.UpdateFilter{}
	filter.UUID = &uuid

	objects, err := GetUpdates(ctx, db, filter)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch from \"updates\" table: %w", err)
	}

	switch len(objects) {
	case 0:
		return nil, ErrNotFound
	case 1:
		return &objects[0], nil
	default:
		return nil, fmt.Errorf("More than one \"updates\" entry matches")
	}
}

// updateColumns returns a string of column names to be used with a SELECT statement for the entity.
// Use this function when building statements to retrieve database entries matching the Update entity.
func updateColumns() string {
	return "updates.id, updates.uuid, updates.origin, updates.external_id, updates.version, updates.published_at, updates.severity, updates.channel, updates.changelog, updates.files, updates.url"
}

// getUpdates can be used to run handwritten sql.Stmts to return a slice of objects.
func getUpdates(ctx context.Context, stmt *sql.Stmt, args ...any) ([]provisioning.Update, error) {
	objects := make([]provisioning.Update, 0)

	dest := func(scan func(dest ...any) error) error {
		u := provisioning.Update{}
		err := scan(&u.ID, &u.UUID, &u.Origin, &u.ExternalID, &u.Version, &u.PublishedAt, &u.Severity, &u.Channel, &u.Changelog, &u.Files, &u.URL)
		if err != nil {
			return err
		}

		objects = append(objects, u)

		return nil
	}

	err := selectObjects(ctx, stmt, dest, args...)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch from \"updates\" table: %w", err)
	}

	return objects, nil
}

// getUpdatesRaw can be used to run handwritten query strings to return a slice of objects.
func getUpdatesRaw(ctx context.Context, db dbtx, sql string, args ...any) ([]provisioning.Update, error) {
	objects := make([]provisioning.Update, 0)

	dest := func(scan func(dest ...any) error) error {
		u := provisioning.Update{}
		err := scan(&u.ID, &u.UUID, &u.Origin, &u.ExternalID, &u.Version, &u.PublishedAt, &u.Severity, &u.Channel, &u.Changelog, &u.Files, &u.URL)
		if err != nil {
			return err
		}

		objects = append(objects, u)

		return nil
	}

	err := scan(ctx, db, sql, dest, args...)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch from \"updates\" table: %w", err)
	}

	return objects, nil
}

// GetUpdates returns all available updates.
// generator: update GetMany
func GetUpdates(ctx context.Context, db dbtx, filters ...provisioning.UpdateFilter) (_ []provisioning.Update, _err error) {
	defer func() {
		_err = mapErr(_err, "Update")
	}()

	var err error

	// Result slice.
	objects := make([]provisioning.Update, 0)

	// Pick the prepared statement and arguments to use based on active criteria.
	var sqlStmt *sql.Stmt
	args := []any{}
	queryParts := [2]string{}

	if len(filters) == 0 {
		sqlStmt, err = Stmt(db, updateObjects)
		if err != nil {
			return nil, fmt.Errorf("Failed to get \"updateObjects\" prepared statement: %w", err)
		}
	}

	for i, filter := range filters {
		if filter.UUID != nil && filter.Channel == nil && filter.Origin == nil {
			args = append(args, []any{filter.UUID}...)
			if len(filters) == 1 {
				sqlStmt, err = Stmt(db, updateObjectsByUUID)
				if err != nil {
					return nil, fmt.Errorf("Failed to get \"updateObjectsByUUID\" prepared statement: %w", err)
				}

				break
			}

			query, err := StmtString(updateObjectsByUUID)
			if err != nil {
				return nil, fmt.Errorf("Failed to get \"updateObjects\" prepared statement: %w", err)
			}

			parts := strings.SplitN(query, "ORDER BY", 2)
			if i == 0 {
				copy(queryParts[:], parts)
				continue
			}

			_, where, _ := strings.Cut(parts[0], "WHERE")
			queryParts[0] += "OR" + where
		} else if filter.Origin != nil && filter.UUID == nil && filter.Channel == nil {
			args = append(args, []any{filter.Origin}...)
			if len(filters) == 1 {
				sqlStmt, err = Stmt(db, updateObjectsByOrigin)
				if err != nil {
					return nil, fmt.Errorf("Failed to get \"updateObjectsByOrigin\" prepared statement: %w", err)
				}

				break
			}

			query, err := StmtString(updateObjectsByOrigin)
			if err != nil {
				return nil, fmt.Errorf("Failed to get \"updateObjects\" prepared statement: %w", err)
			}

			parts := strings.SplitN(query, "ORDER BY", 2)
			if i == 0 {
				copy(queryParts[:], parts)
				continue
			}

			_, where, _ := strings.Cut(parts[0], "WHERE")
			queryParts[0] += "OR" + where
		} else if filter.Channel != nil && filter.UUID == nil && filter.Origin == nil {
			args = append(args, []any{filter.Channel}...)
			if len(filters) == 1 {
				sqlStmt, err = Stmt(db, updateObjectsByChannel)
				if err != nil {
					return nil, fmt.Errorf("Failed to get \"updateObjectsByChannel\" prepared statement: %w", err)
				}

				break
			}

			query, err := StmtString(updateObjectsByChannel)
			if err != nil {
				return nil, fmt.Errorf("Failed to get \"updateObjects\" prepared statement: %w", err)
			}

			parts := strings.SplitN(query, "ORDER BY", 2)
			if i == 0 {
				copy(queryParts[:], parts)
				continue
			}

			_, where, _ := strings.Cut(parts[0], "WHERE")
			queryParts[0] += "OR" + where
		} else if filter.UUID == nil && filter.Channel == nil && filter.Origin == nil {
			return nil, fmt.Errorf("Cannot filter on empty UpdateFilter")
		} else {
			return nil, errors.New("No statement exists for the given Filter")
		}
	}

	// Select.
	if sqlStmt != nil {
		objects, err = getUpdates(ctx, sqlStmt, args...)
	} else {
		queryStr := strings.Join(queryParts[:], "ORDER BY")
		objects, err = getUpdatesRaw(ctx, db, queryStr, args...)
	}

	if err != nil {
		return nil, fmt.Errorf("Failed to fetch from \"updates\" table: %w", err)
	}

	return objects, nil
}

// GetUpdateNames returns the identifying field of update.
// generator: update GetNames
func GetUpdateNames(ctx context.Context, db dbtx, filters ...provisioning.UpdateFilter) (_ []uuid.UUID, _err error) {
	defer func() {
		_err = mapErr(_err, "Update")
	}()

	var err error

	// Result slice.
	names := make([]uuid.UUID, 0)

	// Pick the prepared statement and arguments to use based on active criteria.
	var sqlStmt *sql.Stmt
	args := []any{}
	queryParts := [2]string{}

	if len(filters) == 0 {
		sqlStmt, err = Stmt(db, updateNames)
		if err != nil {
			return nil, fmt.Errorf("Failed to get \"updateNames\" prepared statement: %w", err)
		}
	}

	for i, filter := range filters {
		if filter.Channel != nil && filter.UUID == nil && filter.Origin == nil {
			args = append(args, []any{filter.Channel}...)
			if len(filters) == 1 {
				sqlStmt, err = Stmt(db, updateNamesByChannel)
				if err != nil {
					return nil, fmt.Errorf("Failed to get \"updateNamesByChannel\" prepared statement: %w", err)
				}

				break
			}

			query, err := StmtString(updateNamesByChannel)
			if err != nil {
				return nil, fmt.Errorf("Failed to get \"updateNames\" prepared statement: %w", err)
			}

			parts := strings.SplitN(query, "ORDER BY", 2)
			if i == 0 {
				copy(queryParts[:], parts)
				continue
			}

			_, where, _ := strings.Cut(parts[0], "WHERE")
			queryParts[0] += "OR" + where
		} else if filter.UUID == nil && filter.Channel == nil && filter.Origin == nil {
			return nil, fmt.Errorf("Cannot filter on empty UpdateFilter")
		} else {
			return nil, errors.New("No statement exists for the given Filter")
		}
	}

	// Select.
	var rows *sql.Rows
	if sqlStmt != nil {
		rows, err = sqlStmt.QueryContext(ctx, args...)
	} else {
		queryStr := strings.Join(queryParts[:], "ORDER BY")
		rows, err = db.QueryContext(ctx, queryStr, args...)
	}

	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var identifier uuid.UUID
		err := rows.Scan(&identifier)
		if err != nil {
			return nil, err
		}

		names = append(names, identifier)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch from \"updates\" table: %w", err)
	}

	return names, nil
}

// CreateUpdate adds a new update to the database.
// generator: update Create
func CreateUpdate(ctx context.Context, db dbtx, object provisioning.Update) (_ int64, _err error) {
	defer func() {
		_err = mapErr(_err, "Update")
	}()

	args := make([]any, 10)

	// Populate the statement arguments.
	args[0] = object.UUID
	args[1] = object.Origin
	args[2] = object.ExternalID
	args[3] = object.Version
	args[4] = object.PublishedAt
	args[5] = object.Severity
	args[6] = object.Channel
	args[7] = object.Changelog
	args[8] = object.Files
	args[9] = object.URL

	// Prepared statement to use.
	stmt, err := Stmt(db, updateCreate)
	if err != nil {
		return -1, fmt.Errorf("Failed to get \"updateCreate\" prepared statement: %w", err)
	}

	// Execute the statement.
	result, err := stmt.Exec(args...)
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		if sqliteErr.Code == sqlite3.ErrConstraint {
			return -1, ErrConflict
		}
	}

	if err != nil {
		return -1, fmt.Errorf("Failed to create \"updates\" entry: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return -1, fmt.Errorf("Failed to fetch \"updates\" entry ID: %w", err)
	}

	return id, nil
}

// UpdateUpdate updates the update matching the given key parameters.
// generator: update Update
func UpdateUpdate(ctx context.Context, db tx, uuid uuid.UUID, object provisioning.Update) (_err error) {
	defer func() {
		_err = mapErr(_err, "Update")
	}()

	id, err := GetUpdateID(ctx, db, uuid)
	if err != nil {
		return err
	}

	stmt, err := Stmt(db, updateUpdate)
	if err != nil {
		return fmt.Errorf("Failed to get \"updateUpdate\" prepared statement: %w", err)
	}

	result, err := stmt.Exec(object.UUID, object.Origin, object.ExternalID, object.Version, object.PublishedAt, object.Severity, object.Channel, object.Changelog, object.Files, object.URL, id)
	if err != nil {
		return fmt.Errorf("Update \"updates\" entry failed: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("Fetch affected rows: %w", err)
	}

	if n != 1 {
		return fmt.Errorf("Query updated %d rows instead of 1", n)
	}

	return nil
}

// DeleteUpdate deletes the update matching the given key parameters.
// generator: update DeleteOne-by-UUID
func DeleteUpdate(ctx context.Context, db dbtx, uuid uuid.UUID) (_err error) {
	defer func() {
		_err = mapErr(_err, "Update")
	}()

	stmt, err := Stmt(db, updateDeleteByUUID)
	if err != nil {
		return fmt.Errorf("Failed to get \"updateDeleteByUUID\" prepared statement: %w", err)
	}

	result, err := stmt.Exec(uuid)
	if err != nil {
		return fmt.Errorf("Delete \"updates\": %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("Fetch affected rows: %w", err)
	}

	if n == 0 {
		return ErrNotFound
	} else if n > 1 {
		return fmt.Errorf("Query deleted %d Update rows instead of 1", n)
	}

	return nil
}
