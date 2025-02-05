package jetdb

import (
	"context"

	. "github.com/go-jet/jet/v2/sqlite"
	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/operations"
	"github.com/FuturFusion/operations-center/internal/operations/repo"
	"github.com/FuturFusion/operations-center/internal/operations/repo/jetdb/.gen/model"
	. "github.com/FuturFusion/operations-center/internal/operations/repo/jetdb/.gen/table"
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
	stmt := Tokens.INSERT(
		Tokens.UUID, Tokens.UsesRemaining, Tokens.ExpireAt, Tokens.Description,
	).VALUES(
		in.UUID, in.UsesRemaining, in.ExpireAt, in.Description,
	).RETURNING(
		Tokens.UUID, Tokens.UsesRemaining, Tokens.ExpireAt, Tokens.Description,
	)

	var token model.Tokens
	err := stmt.QueryContext(ctx, t.db, &token)
	if err != nil {
		return operations.Token{}, err
	}

	return operations.Token(token), nil
}

func (t token) GetAll(ctx context.Context) (operations.Tokens, error) {
	stmt := SELECT(
		Tokens.UUID, Tokens.UsesRemaining, Tokens.ExpireAt, Tokens.Description,
	).FROM(
		Tokens,
	)

	var tokens []model.Tokens
	err := stmt.QueryContext(ctx, t.db, &tokens)
	if err != nil {
		return nil, err
	}

	return toTokens(tokens), nil
}

func (t token) GetAllIDs(ctx context.Context) ([]string, error) {
	stmt := SELECT(
		Tokens.UUID,
	).FROM(
		Tokens,
	).ORDER_BY(
		Tokens.UUID,
	)

	var tokens []string
	err := stmt.QueryContext(ctx, t.db, &tokens)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (t token) GetByID(ctx context.Context, id uuid.UUID) (operations.Token, error) {
	stmt := SELECT(
		Tokens.UUID, Tokens.UsesRemaining, Tokens.ExpireAt, Tokens.Description,
	).FROM(
		Tokens,
	).WHERE(
		Tokens.UUID.EQ(UUID(id)),
	)

	var token model.Tokens
	err := stmt.QueryContext(ctx, t.db, &token)
	if err != nil {
		return operations.Token{}, err
	}

	return operations.Token(token), nil
}

func (t token) UpdateByID(ctx context.Context, in operations.Token) (operations.Token, error) {
	stmt := Tokens.UPDATE(
		Tokens.UsesRemaining, Tokens.ExpireAt, Tokens.Description,
	).SET(
		in.UsesRemaining, in.ExpireAt, in.Description,
	).WHERE(
		Tokens.UUID.EQ(UUID(in.UUID)),
	).RETURNING(
		Tokens.UUID, Tokens.UsesRemaining, Tokens.ExpireAt, Tokens.Description,
	)

	var token model.Tokens
	err := stmt.QueryContext(ctx, t.db, &token)
	if err != nil {
		return operations.Token{}, err
	}

	return operations.Token(token), nil
}

func (t token) DeleteByID(ctx context.Context, id uuid.UUID) error {
	stmt := Tokens.DELETE().WHERE(
		Tokens.UUID.EQ(UUID(id)),
	)

	result, err := stmt.ExecContext(ctx, t.db)
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

func toTokens(in []model.Tokens) operations.Tokens {
	tokens := make(operations.Tokens, 0, len(in))
	for _, item := range in {
		tokens = append(tokens, operations.Token(item))
	}

	return tokens
}
