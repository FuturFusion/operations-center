package provisioning

import (
	"context"

	"github.com/google/uuid"
)

type TokenService interface {
	Create(ctx context.Context, token Token) (Token, error)
	GetAll(ctx context.Context) (Tokens, error)
	GetAllIDs(ctx context.Context) ([]uuid.UUID, error)
	GetByID(ctx context.Context, id uuid.UUID) (Token, error)
	UpdateByID(ctx context.Context, token Token) (Token, error)
	DeleteByID(ctx context.Context, id uuid.UUID) error
}

type TokenRepo interface {
	Create(ctx context.Context, token Token) (Token, error)
	GetAll(ctx context.Context) (Tokens, error)
	GetAllIDs(ctx context.Context) ([]uuid.UUID, error)
	GetByID(ctx context.Context, id uuid.UUID) (Token, error)
	UpdateByID(ctx context.Context, token Token) (Token, error)
	DeleteByID(ctx context.Context, id uuid.UUID) error
}
