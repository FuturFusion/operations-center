package sqlite

import (
	"context"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/sqlite"
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

func (t token) CreateTokenSeed(ctx context.Context, in provisioning.TokenSeed) (int64, error) {
	return entities.CreateTokenSeed(ctx, transaction.GetDBTX(ctx, t.db), in)
}

func (t token) GetTokenSeedAll(ctx context.Context, id uuid.UUID) (provisioning.TokenSeeds, error) {
	return entities.GetTokenSeeds(ctx, transaction.GetDBTX(ctx, t.db), entities.TokenSeedFilter{
		Token: &id,
	})
}

func (t token) GetTokenSeedAllNames(ctx context.Context, id uuid.UUID) ([]string, error) {
	tokenSeeds, err := entities.GetTokenSeeds(ctx, transaction.GetDBTX(ctx, t.db), entities.TokenSeedFilter{
		Token: &id,
	})
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(tokenSeeds))
	for _, tokenSeed := range tokenSeeds {
		names = append(names, tokenSeed.Name)
	}

	return names, nil
}

func (t token) GetTokenSeedByName(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
	return entities.GetTokenSeed(ctx, transaction.GetDBTX(ctx, t.db), id, name)
}

func (t token) UpdateTokenSeed(ctx context.Context, in provisioning.TokenSeed) error {
	return transaction.ForceTx(ctx, transaction.GetDBTX(ctx, t.db), func(ctx context.Context, tx transaction.TX) error {
		return entities.UpdateTokenSeed(ctx, tx, in.Token, in.Name, in)
	})
}

func (t token) DeleteTokenSeedByName(ctx context.Context, id uuid.UUID, name string) error {
	return entities.DeleteTokenSeed(ctx, transaction.GetDBTX(ctx, t.db), name, id)
}
