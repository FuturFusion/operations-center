package provisioning

import (
	"context"
	"io"

	"github.com/google/uuid"
)

type TokenService interface {
	Create(ctx context.Context, token Token) (Token, error)
	GetAll(ctx context.Context) (Tokens, error)
	GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Token, error)
	Update(ctx context.Context, token Token) error
	DeleteByUUID(ctx context.Context, id uuid.UUID) error
	Consume(ctx context.Context, id uuid.UUID) error
	GetPreSeedImage(ctx context.Context, id uuid.UUID, seedConfig TokenSeedConfig) (_ io.ReadCloser, _ error)
}

type TokenRepo interface {
	Create(ctx context.Context, token Token) (int64, error)
	GetAll(ctx context.Context) (Tokens, error)
	GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Token, error)
	Update(ctx context.Context, token Token) error
	DeleteByUUID(ctx context.Context, id uuid.UUID) error
}

type FlasherPort interface {
	GenerateSeededImage(ctx context.Context, id uuid.UUID, seedConfig TokenSeedConfig, rc io.ReadCloser) (_ io.ReadCloser, _ error)
}
