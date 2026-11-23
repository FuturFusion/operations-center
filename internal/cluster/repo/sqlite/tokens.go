package sqlite

import (
	"context"
	"errors"
	"time"

	"github.com/cowsql/go-cowsql/cluster/db/transaction"

	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/domain"
)

type joinTokenRepo struct {
	db transaction.Transactor
}

func NewJoinTokenRepo(db transaction.Transactor) *joinTokenRepo {
	return &joinTokenRepo{db: db}
}

// GetTokens returns the tokens, which have not expired. Expired tokens are removed by the next change of the tokens, so reads don't write.
func (g *joinTokenRepo) GetTokens(ctx context.Context) ([]entities.ClusterJoinToken, error) {
	var tokens []entities.ClusterJoinToken
	err := transaction.ForceTx(ctx, g.db, func(ctx context.Context, tx transaction.TX) error {
		var err error
		tokens, err = entities.GetClusterJoinTokens(ctx, tx)

		return err
	})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	valid := make([]entities.ClusterJoinToken, 0, len(tokens))
	for _, token := range tokens {
		if !token.Expired(now) {
			valid = append(valid, token)
		}
	}

	return valid, nil
}

// GetTokenByServerName returns the token of serverName, an expired one counts as not found.
func (g *joinTokenRepo) GetTokenByServerName(ctx context.Context, serverName string) (*entities.ClusterJoinToken, error) {
	var token *entities.ClusterJoinToken
	err := transaction.ForceTx(ctx, g.db, func(ctx context.Context, tx transaction.TX) error {
		var err error
		token, err = entities.GetClusterJoinToken(ctx, tx, serverName)

		return err
	})
	if err != nil {
		return nil, mapTokenError(err, serverName)
	}

	if token.Expired(time.Now()) {
		return nil, mapTokenError(entities.ErrNotFound, serverName)
	}

	return token, nil
}

func (g *joinTokenRepo) CreateToken(ctx context.Context, token entities.ClusterJoinToken) error {
	return transaction.ForceTx(ctx, g.db, func(ctx context.Context, tx transaction.TX) error {
		err := entities.DeleteExpiredTokens(ctx, tx)
		if err != nil {
			return err
		}

		_, err = entities.CreateClusterJoinToken(ctx, tx, token)

		return mapTokenError(err, token.ServerName)
	})
}

// SetJoinerFingerprint records the fingerprint of the certificate of the server, which presented the token of serverName.
func (g *joinTokenRepo) SetJoinerFingerprint(ctx context.Context, serverName string, fingerprint string) error {
	return transaction.ForceTx(ctx, g.db, func(ctx context.Context, tx transaction.TX) error {
		token, err := entities.GetClusterJoinToken(ctx, tx, serverName)
		if err != nil {
			return mapTokenError(err, serverName)
		}

		token.JoinerFingerprint = fingerprint

		return mapTokenError(entities.UpdateClusterJoinToken(ctx, tx, serverName, *token), serverName)
	})
}

func (g *joinTokenRepo) DeleteTokenByName(ctx context.Context, serverName string) error {
	return transaction.ForceTx(ctx, g.db, func(ctx context.Context, tx transaction.TX) error {
		err := entities.DeleteExpiredTokens(ctx, tx)
		if err != nil {
			return err
		}

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
