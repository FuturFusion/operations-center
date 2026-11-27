package sqlite_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/sql/cowsql/cowsqltest"
)

func TestJoinTokenRepo(t *testing.T) {
	t.Parallel()

	db, err := cowsqltest.Open(t)
	require.NoError(t, err)

	stmts, err := entities.PrepareStmts(db, false)
	require.NoError(t, err)

	ctx := entities.WithPreparedStmts(t.Context(), stmts)
	repo := sqlite.NewJoinTokenRepo(sqlite.NewGlobalRaftRepo(db))

	valid := entities.ClusterJoinToken{ServerName: "m2", Secret: "secret-m2", ClusterFingerprint: "fp", ClusterAddresses: []string{"192.0.2.1:8443"}, ExpirationDate: time.Now().Add(time.Hour)}
	expired := entities.ClusterJoinToken{ServerName: "m3", Secret: "secret-m3", ClusterFingerprint: "fp", ClusterAddresses: []string{"192.0.2.1:8443"}, ExpirationDate: time.Now().Add(-time.Minute)}

	require.NoError(t, repo.CreateToken(ctx, valid))
	require.NoError(t, repo.CreateToken(ctx, expired))

	// A second token for the same server is refused.
	err = repo.CreateToken(ctx, valid)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)

	// Expired tokens are neither listed nor found, but reads keep them in the database.
	tokens, err := repo.GetTokens(ctx)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.Equal(t, "m2", tokens[0].ServerName)

	_, err = repo.GetTokenByServerName(ctx, "m3")
	require.ErrorIs(t, err, domain.ErrNotFound)

	_, err = repo.GetTokenByServerName(ctx, "unknown")
	require.ErrorIs(t, err, domain.ErrNotFound)

	var stored int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cluster_join_tokens`).Scan(&stored))
	require.Equal(t, 2, stored)

	// The joiner fingerprint is recorded with the token.
	require.NoError(t, repo.SetJoinerFingerprint(ctx, "m2", "joiner-fp"))

	token, err := repo.GetTokenByServerName(ctx, "m2")
	require.NoError(t, err)
	require.Equal(t, "joiner-fp", token.JoinerFingerprint)
	require.Equal(t, "secret-m2", token.Secret)

	err = repo.SetJoinerFingerprint(ctx, "unknown", "joiner-fp")
	require.ErrorIs(t, err, domain.ErrNotFound)

	// A change of the tokens removes the expired ones.
	require.NoError(t, repo.DeleteTokenByName(ctx, "m2"))

	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cluster_join_tokens`).Scan(&stored))
	require.Zero(t, stored)

	err = repo.DeleteTokenByName(ctx, "m2")
	require.ErrorIs(t, err, domain.ErrNotFound)
}
