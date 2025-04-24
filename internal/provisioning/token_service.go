package provisioning

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/transaction"
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

	newToken.ID, err = s.repo.Create(ctx, newToken)
	if err != nil {
		return Token{}, err
	}

	return newToken, nil
}

func (s tokenService) GetAll(ctx context.Context) (Tokens, error) {
	return s.repo.GetAll(ctx)
}

func (s tokenService) GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error) {
	return s.repo.GetAllUUIDs(ctx)
}

func (s tokenService) GetByUUID(ctx context.Context, id uuid.UUID) (*Token, error) {
	return s.repo.GetByUUID(ctx, id)
}

func (s tokenService) Update(ctx context.Context, newToken Token) error {
	err := newToken.Validate()
	if err != nil {
		return err
	}

	return s.repo.Update(ctx, newToken)
}

func (s tokenService) DeleteByUUID(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteByUUID(ctx, id)
}

func (s tokenService) Consume(ctx context.Context, id uuid.UUID) error {
	return transaction.Do(ctx, func(ctx context.Context) error {
		token, err := s.repo.GetByUUID(ctx, id)
		if err != nil {
			return fmt.Errorf("Consume token: %w", err)
		}

		if token.UsesRemaining < 1 {
			return fmt.Errorf("Token exhausted")
		}

		if time.Now().After(token.ExpireAt) {
			return fmt.Errorf("Token expired")
		}

		token.UsesRemaining--

		err = s.repo.Update(ctx, *token)
		if err != nil {
			return fmt.Errorf("Update token: %w", err)
		}

		return nil
	})
}
