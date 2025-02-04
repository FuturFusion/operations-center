package sqlite

import (
	"context"
	"database/sql"
	"errors"

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
