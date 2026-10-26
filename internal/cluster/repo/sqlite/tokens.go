package sqlite

import (
	"context"
	"errors"

	"github.com/cowsql/go-cowsql/cluster/db/transaction"

	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/domain"
)

type tokenRepo struct {
	db transaction.Transactor
}

func NewTokenRepo(db transaction.Transactor) *tokenRepo {
	return &tokenRepo{db: db}
}

func (g *tokenRepo) GetTokens(ctx context.Context) ([]entities.ClusterJoinToken, error) {
	var tokens []entities.ClusterJoinToken
	err := transaction.ForceTx(ctx, g.db, func(ctx context.Context, tx transaction.TX) error {
		err := entities.DeleteExpiredTokens(ctx, tx)
		if err != nil {
			return err
		}

		tokens, err = entities.GetClusterJoinTokens(ctx, tx)

		return err
	})
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (g *tokenRepo) GetTokenByServerName(ctx context.Context, serverName string) (*entities.ClusterJoinToken, error) {
	var token *entities.ClusterJoinToken
	err := transaction.ForceTx(ctx, g.db, func(ctx context.Context, tx transaction.TX) error {
		err := entities.DeleteExpiredTokens(ctx, tx)
		if err != nil {
			return err
		}

		token, err = entities.GetClusterJoinToken(ctx, tx, serverName)

		return err
	})
	if err != nil {
		return nil, mapTokenError(err, serverName)
	}

	return token, nil
}

func (g *tokenRepo) CreateToken(ctx context.Context, token entities.ClusterJoinToken) error {
	return transaction.ForceTx(ctx, g.db, func(ctx context.Context, tx transaction.TX) error {
		err := entities.DeleteExpiredTokens(ctx, tx)
		if err != nil {
			return err
		}

		_, err = entities.CreateClusterJoinToken(ctx, tx, token)

		return mapTokenError(err, token.ServerName)
	})
}

func (g *tokenRepo) DeleteTokenByName(ctx context.Context, serverName string) error {
	return transaction.ForceTx(ctx, g.db, func(ctx context.Context, tx transaction.TX) error {
		return mapTokenError(entities.DeleteClusterJoinToken(ctx, tx, serverName), serverName)
	})
}

// mapTokenError turns the entity errors into the errors reported to the user.
func mapTokenError(err error, serverName string) error {
	if errors.Is(err, entities.ErrNotFound) {
		return domain.NewErrorf(domain.ErrNotFound, "", "No join token for server %q", serverName)
	}

	if errors.Is(err, entities.ErrConflict) {
		return domain.NewErrorf(domain.ErrConstraintViolation, "", "A join token for server %q exists already", serverName)
	}

	return err
}
