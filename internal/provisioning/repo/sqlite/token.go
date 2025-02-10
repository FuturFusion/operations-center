package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo"
)

type token struct {
	db repo.DBTX
}

var _ provisioning.TokenRepo = &token{}

func NewToken(db repo.DBTX) *token {
	return &token{
		db: db,
	}
}

func (t token) Create(ctx context.Context, in provisioning.Token) (provisioning.Token, error) {
	const sqlStmt = `
INSERT INTO tokens (uuid, uses_remaining, expire_at, description)
VALUES(:uuid, :uses_remaining, :expire_at, :description)
RETURNING uuid, uses_remaining, expire_at, description;
`

	row := t.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("uuid", in.UUID),
		sql.Named("uses_remaining", in.UsesRemaining),
		sql.Named("expire_at", datetime(in.ExpireAt)),
		sql.Named("description", in.Description),
	)
	if row.Err() != nil {
		return provisioning.Token{}, mapErr(row.Err())
	}

	return scanToken(row)
}

func (t token) GetAll(ctx context.Context) (provisioning.Tokens, error) {
	const sqlStmt = `SELECT uuid, uses_remaining, expire_at, description FROM tokens;`

	rows, err := t.db.QueryContext(ctx, sqlStmt)
	if err != nil {
		return nil, mapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var tokens provisioning.Tokens
	for rows.Next() {
		token, err := scanToken(rows)
		if err != nil {
			return nil, mapErr(err)
		}

		tokens = append(tokens, token)
	}

	if rows.Err() != nil {
		return nil, mapErr(rows.Err())
	}

	return tokens, nil
}

func (t token) GetAllIDs(ctx context.Context) ([]string, error) {
	const sqlStmt = `SELECT uuid FROM tokens ORDER BY uuid`

	rows, err := t.db.QueryContext(ctx, sqlStmt)
	if err != nil {
		return nil, mapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var tokenIDs []string
	for rows.Next() {
		var tokenID string
		err := rows.Scan(&tokenID)
		if err != nil {
			return nil, mapErr(err)
		}

		tokenIDs = append(tokenIDs, tokenID)
	}

	if rows.Err() != nil {
		return nil, mapErr(rows.Err())
	}

	return tokenIDs, nil
}

func (t token) GetByID(ctx context.Context, id uuid.UUID) (provisioning.Token, error) {
	const sqlStmt = `SELECT uuid, uses_remaining, expire_at, description FROM tokens WHERE uuid=:uuid;`

	row := t.db.QueryRowContext(ctx, sqlStmt, sql.Named("uuid", id))
	if row.Err() != nil {
		return provisioning.Token{}, mapErr(row.Err())
	}

	return scanToken(row)
}

func (t token) UpdateByID(ctx context.Context, in provisioning.Token) (provisioning.Token, error) {
	const sqlStmt = `
UPDATE tokens SET uses_remaining=:uses_remaining, expire_at=:expire_at, description=:description
WHERE uuid=:uuid
RETURNING uuid, uses_remaining, expire_at, description;
`

	row := t.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("uses_remaining", in.UsesRemaining),
		sql.Named("expire_at", datetime(in.ExpireAt)),
		sql.Named("description", in.Description),
		sql.Named("uuid", in.UUID),
	)
	if row.Err() != nil {
		return provisioning.Token{}, mapErr(row.Err())
	}

	return scanToken(row)
}

func (t token) DeleteByID(ctx context.Context, id uuid.UUID) error {
	const sqlStmt = `DELETE FROM tokens WHERE uuid=:uuid;`

	result, err := t.db.ExecContext(ctx, sqlStmt, sql.Named("uuid", id))
	if err != nil {
		return mapErr(err)
	}

	affectedRows, err := result.RowsAffected()
	if err != nil {
		return mapErr(err)
	}

	if affectedRows == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func scanToken(row interface{ Scan(dest ...any) error }) (provisioning.Token, error) {
	var token provisioning.Token
	var expireAt datetime

	err := row.Scan(
		&token.UUID,
		&token.UsesRemaining,
		&expireAt,
		&token.Description,
	)
	if err != nil {
		return provisioning.Token{}, mapErr(err)
	}

	token.ExpireAt = time.Time(expireAt)

	return token, nil
}
