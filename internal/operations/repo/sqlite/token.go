package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"

	"github.com/FuturFusion/operations-center/internal/operations"
	"github.com/FuturFusion/operations-center/internal/operations/repo"
)

type token struct {
	db repo.DBTX
}

var _ operations.TokenRepo = &token{}

func NewToken(db repo.DBTX) *token {
	return &token{
		db: db,
	}
}

func (t token) Create(ctx context.Context, in operations.Token) (operations.Token, error) {
	const sqlInsert = `
INSERT INTO tokens (uuid, uses_remaining, expire_at, description)
VALUES(:uuid, :uses_remaining, :expire_at, :description)
RETURNING uuid, uses_remaining, expire_at, description;
`

	marshalledExpireAt, err := in.ExpireAt.MarshalText()
	if err != nil {
		return operations.Token{}, err
	}

	row := t.db.QueryRowContext(ctx, sqlInsert,
		sql.Named("uuid", in.UUID),
		sql.Named("uses_remaining", in.UsesRemaining),
		sql.Named("expire_at", marshalledExpireAt),
		sql.Named("description", in.Description),
	)
	if row.Err() != nil {
		return operations.Token{}, row.Err()
	}

	return scanToken(row)
}

func (t token) GetAll(ctx context.Context) (operations.Tokens, error) {
	const sqlGetAll = `SELECT uuid, uses_remaining, expire_at, description FROM tokens;`

	rows, err := t.db.QueryContext(ctx, sqlGetAll)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var tokens operations.Tokens
	for rows.Next() {
		token, err := scanToken(rows)
		if err != nil {
			return nil, err
		}

		tokens = append(tokens, token)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return tokens, nil
}

func (t token) GetAllIDs(ctx context.Context) ([]string, error) {
	const sqlGetAllIDs = `SELECT uuid FROM tokens ORDER BY uuid`

	rows, err := t.db.QueryContext(ctx, sqlGetAllIDs)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var tokenIDs []string
	for rows.Next() {
		var tokenID string
		err := rows.Scan(&tokenID)
		if err != nil {
			return nil, err
		}

		tokenIDs = append(tokenIDs, tokenID)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return tokenIDs, nil
}

func (t token) GetByID(ctx context.Context, id uuid.UUID) (operations.Token, error) {
	const sqlGetByID = `SELECT uuid, uses_remaining, expire_at, description FROM tokens WHERE uuid=:uuid;`

	row := t.db.QueryRowContext(ctx, sqlGetByID, sql.Named("uuid", id))
	if row.Err() != nil {
		return operations.Token{}, row.Err()
	}

	return scanToken(row)
}

func (t token) UpdateByID(ctx context.Context, in operations.Token) (operations.Token, error) {
	const sqlUpdate = `
UPDATE tokens SET uses_remaining=:uses_remaining, expire_at=:expire_at, description=:description
WHERE uuid=:uuid
RETURNING uuid, uses_remaining, expire_at, description;
`

	marshalledExpireAt, err := in.ExpireAt.MarshalText()
	if err != nil {
		return operations.Token{}, err
	}

	row := t.db.QueryRowContext(ctx, sqlUpdate,
		sql.Named("uses_remaining", in.UsesRemaining),
		sql.Named("expire_at", marshalledExpireAt),
		sql.Named("description", in.Description),
		sql.Named("uuid", in.UUID),
	)
	if row.Err() != nil {
		return operations.Token{}, row.Err()
	}

	return scanToken(row)
}

func (t token) DeleteByID(ctx context.Context, id uuid.UUID) error {
	const sqlDelete = `DELETE FROM tokens WHERE uuid=:uuid;`

	result, err := t.db.ExecContext(ctx, sqlDelete, sql.Named("uuid", id))
	if err != nil {
		return err
	}

	affectedRows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if affectedRows == 0 {
		return operations.ErrNotFound
	}

	return nil
}

func scanToken(row interface{ Scan(dest ...any) error }) (operations.Token, error) {
	var token operations.Token
	var marshalledExpireAt string
	err := row.Scan(
		&token.UUID,
		&token.UsesRemaining,
		&marshalledExpireAt,
		&token.Description,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return operations.Token{}, operations.ErrNotFound
		}

		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) {
			if sqliteErr.Code == sqlite3.ErrConstraint {
				return operations.Token{}, operations.ErrConstraintViolation
			}
		}

		return operations.Token{}, err
	}

	err = token.ExpireAt.UnmarshalText([]byte(marshalledExpireAt))
	if err != nil {
		return operations.Token{}, err
	}

	return token, nil
}
