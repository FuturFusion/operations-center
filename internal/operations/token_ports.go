package operations

import (
	"context"

	"github.com/google/uuid"
)

type TokenService interface {
	Create(ctx context.Context, token Token) (Token, error)
	GetAll(ctx context.Context) (Tokens, error)
	GetAllIDs(ctx context.Context) ([]string, error)
	GetByID(ctx context.Context, id uuid.UUID) (Token, error)
	UpdateByID(ctx context.Context, token Token) (Token, error)
	DeleteByID(ctx context.Context, id uuid.UUID) error
}

//go:generate go run github.com/matryer/moq -fmt goimports -pkg mock -out repo/mock/token_repo_mock_gen.go -rm . TokenRepo
//go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i TokenRepo -t ../logger/slog.gotmpl -o ./repo/middleware/token_slog_gen.go
// disabled go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i TokenRepo -t prometheus -o ./repo/middleware/token_prometheus_gen.go

type TokenRepo interface {
	Create(ctx context.Context, token Token) (Token, error)
	GetAll(ctx context.Context) (Tokens, error)
	GetAllIDs(ctx context.Context) ([]string, error)
	GetByID(ctx context.Context, id uuid.UUID) (Token, error)
	UpdateByID(ctx context.Context, token Token) (Token, error)
	DeleteByID(ctx context.Context, id uuid.UUID) error
}
