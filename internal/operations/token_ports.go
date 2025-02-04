package operations

import "context"

type TokenService interface {
	Create(ctx context.Context, token Token) (Token, error)
}

//go:generate go run github.com/matryer/moq -fmt goimports -pkg mock -out repo/mock/token_repo_mock_gen.go -rm . TokenRepo
//go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i TokenRepo -t ../logger/slog.gotmpl -o ./repo/middleware/token_slog_gen.go
// disabled go:generate go run github.com/hexdigest/gowrap/cmd/gowrap gen -g -i TokenRepo -t prometheus -o ./repo/middleware/token_prometheus_gen.go

type TokenRepo interface {
	Create(ctx context.Context, token Token) (Token, error)
}
