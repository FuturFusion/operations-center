package provisioning

import (
	"context"

	"github.com/google/uuid"
)

type tokenService struct {
	repo TokenRepo

	randomUUID func() (uuid.UUID, error)
}

var _ TokenService = &tokenService{}

type TokenServiceOption func(s *tokenService)

func NewTokenService(repo TokenRepo, opts ...TokenServiceOption) tokenService {
	tokenSvc := tokenService{
		repo:       repo,
		randomUUID: uuid.NewRandom,
	}

	for _, opt := range opts {
		opt(&tokenSvc)
	}

	return tokenSvc
}

func (s tokenService) Create(ctx context.Context, newToken Token) (Token, error) {
	var err error
	newToken.UUID, err = s.randomUUID()
	if err != nil {
		return Token{}, err
	}

	err = newToken.Validate()
	if err != nil {
		return Token{}, err
	}

	return s.repo.Create(ctx, newToken)
}

func (s tokenService) GetAll(ctx context.Context) (Tokens, error) {
	return s.repo.GetAll(ctx)
}

func (s tokenService) GetAllIDs(ctx context.Context) ([]uuid.UUID, error) {
	return s.repo.GetAllIDs(ctx)
}

func (s tokenService) GetByID(ctx context.Context, id uuid.UUID) (Token, error) {
	return s.repo.GetByID(ctx, id)
}

func (s tokenService) UpdateByID(ctx context.Context, newToken Token) (Token, error) {
	err := newToken.Validate()
	if err != nil {
		return Token{}, err
	}

	return s.repo.UpdateByID(ctx, newToken)
}

func (s tokenService) DeleteByID(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteByID(ctx, id)
}
