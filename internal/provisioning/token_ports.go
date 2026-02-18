package provisioning

import (
	"context"
	"io"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"

	"github.com/FuturFusion/operations-center/shared/api"
)

type TokenService interface {
	Create(ctx context.Context, token Token) (Token, error)
	GetAll(ctx context.Context) (Tokens, error)
	GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*Token, error)
	Update(ctx context.Context, token Token) error
	DeleteByUUID(ctx context.Context, id uuid.UUID) error
	Consume(ctx context.Context, id uuid.UUID) (channel string, _ error)
	PreparePreSeededImage(ctx context.Context, id uuid.UUID, imageType api.ImageType, architecture images.UpdateFileArchitecture, seedConfig TokenImageSeedConfigs) (uuid.UUID, error)
	GetPreSeededImage(ctx context.Context, id uuid.UUID, imageUUID uuid.UUID) (_ io.ReadCloser, filename string, _ error)
	GetTokenProviderConfig(ctx context.Context, id uuid.UUID) (*api.TokenProviderConfig, error)
	CreateTokenSeed(ctx context.Context, tokenSeedConfig TokenSeed) (TokenSeed, error)
	GetTokenSeedAll(ctx context.Context, id uuid.UUID) (TokenSeeds, error)
	GetTokenSeedAllNames(ctx context.Context, id uuid.UUID) ([]string, error)
	GetTokenSeedByName(ctx context.Context, id uuid.UUID, name string) (*TokenSeed, error)
	UpdateTokenSeed(ctx context.Context, tokenSeed TokenSeed) error
	DeleteTokenSeedByName(ctx context.Context, id uuid.UUID, name string) error
	GetTokenImageFromTokenSeed(ctx context.Context, id uuid.UUID, name string, imageType api.ImageType, architecture images.UpdateFileArchitecture, channel string) (io.ReadCloser, error)
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
	GetProviderConfig(ctx context.Context, tokenID uuid.UUID) (*api.TokenProviderConfig, error)
	GenerateSeededImage(ctx context.Context, id uuid.UUID, seedConfig TokenImageSeedConfigs, rc io.ReadCloser) (io.ReadCloser, error)
}
