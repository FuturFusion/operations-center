package provisioning

import (
	"context"
	"io"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/shared/api"
)

type TokenService interface {
	Create(ctx context.Context, token Token) (Token, error)
	GetAll(ctx context.Context) (Tokens, error)
	GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Token, error)
	Update(ctx context.Context, token Token) error
	DeleteByUUID(ctx context.Context, id uuid.UUID) error
	Consume(ctx context.Context, id uuid.UUID) error
	GetPreSeedImage(ctx context.Context, id uuid.UUID, imageType api.ImageType, seedConfig TokenImageSeedConfigs) (io.ReadCloser, error)
	CreateTokenSeed(ctx context.Context, tokenSeedConfig TokenSeed) (TokenSeed, error)
	GetTokenSeedAll(ctx context.Context, id uuid.UUID) (TokenSeeds, error)
	GetTokenSeedAllNames(ctx context.Context, id uuid.UUID) ([]string, error)
	GetTokenSeedByName(ctx context.Context, id uuid.UUID, name string) (*TokenSeed, error)
	UpdateTokenSeed(ctx context.Context, tokenSeed TokenSeed) error
	DeleteTokenSeedByName(ctx context.Context, id uuid.UUID, name string) error
	GetTokenImageFromTokenSeed(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType) (io.ReadCloser, error)
}

type TokenRepo interface {
	Create(ctx context.Context, token Token) (int64, error)
	GetAll(ctx context.Context) (Tokens, error)
	GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Token, error)
	Update(ctx context.Context, token Token) error
	DeleteByUUID(ctx context.Context, id uuid.UUID) error
	CreateTokenSeed(ctx context.Context, seedConfig TokenSeed) (int64, error)
	GetTokenSeedAll(ctx context.Context, id uuid.UUID) (TokenSeeds, error)
	GetTokenSeedAllNames(ctx context.Context, id uuid.UUID) ([]string, error)
	GetTokenSeedByName(ctx context.Context, id uuid.UUID, name string) (*TokenSeed, error)
	UpdateTokenSeed(ctx context.Context, tokenSeedConfig TokenSeed) error
	DeleteTokenSeedByName(ctx context.Context, id uuid.UUID, name string) error
}

type FlasherPort interface {
	GenerateSeededImage(ctx context.Context, id uuid.UUID, seedConfig TokenImageSeedConfigs, rc io.ReadCloser) (io.ReadCloser, error)
}
