package sqlite

import (
	"context"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
)

type token struct {
	db sqlite.DBTX
}

var _ provisioning.TokenRepo = &token{}

func NewToken(db sqlite.DBTX) *token {
	return &token{
		db: db,
	}
}

func (t token) Create(ctx context.Context, in provisioning.Token) (int64, error) {
	return entities.CreateToken(ctx, transaction.GetDBTX(ctx, t.db), in)
}

func (t token) GetAll(ctx context.Context) (provisioning.Tokens, error) {
	return entities.GetTokens(ctx, transaction.GetDBTX(ctx, t.db))
}

func (t token) GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error) {
	return entities.GetTokenNames(ctx, transaction.GetDBTX(ctx, t.db))
}

func (t token) GetByUUID(ctx context.Context, id uuid.UUID) (*provisioning.Token, error) {
	return entities.GetToken(ctx, transaction.GetDBTX(ctx, t.db), id)
}

func (t token) Update(ctx context.Context, in provisioning.Token) error {
	return transaction.ForceTx(ctx, transaction.GetDBTX(ctx, t.db), func(ctx context.Context, tx transaction.TX) error {
		return entities.UpdateToken(ctx, tx, in.UUID, in)
	})
}

func (t token) DeleteByUUID(ctx context.Context, id uuid.UUID) error {
	return entities.DeleteToken(ctx, transaction.GetDBTX(ctx, t.db), id)
}
