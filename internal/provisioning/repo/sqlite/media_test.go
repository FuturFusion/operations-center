package sqlite_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/cowsql/cowsqltest"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
)

func TestMediaDatabaseActions(t *testing.T) {
	t.Parallel()

	db, err := cowsqltest.Open(t)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	tx := transaction.Enable(db)
	stmts, err := entities.PrepareStmts(tx.(transaction.Preparer), false)
	require.NoError(t, err)

	ctx := entities.WithPreparedStmts(t.Context(), stmts)
	repo := sqlite.NewMedia(tx)

	expiresAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedImage := provisioning.Media{Type: provisioning.MediaTypeSeedImage, Name: "image", MemberID: 1, Config: json.RawMessage(`{"seed":"one"}`), ExpiresAt: expiresAt}
	secureBoot := provisioning.Media{Type: provisioning.MediaTypeSecureBoot, Name: "image", MemberID: 1, Config: json.RawMessage(`{}`), ExpiresAt: expiresAt}

	_, err = repo.Create(ctx, seedImage)
	require.NoError(t, err)

	// The same name is used by media of another type.
	_, err = repo.Create(ctx, secureBoot)
	require.NoError(t, err)

	_, err = repo.Create(ctx, seedImage)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)

	got, err := repo.GetByTypeAndName(ctx, provisioning.MediaTypeSeedImage, "image")
	require.NoError(t, err)
	require.JSONEq(t, `{"seed":"one"}`, string(got.Config))
	require.True(t, got.ExpiresAt.Equal(expiresAt))

	got.ExpiresAt = expiresAt.Add(time.Hour)
	require.NoError(t, repo.Update(ctx, *got))

	medias, err := repo.GetAllWithFilter(ctx, provisioning.MediaFilter{Type: new(provisioning.MediaTypeSeedImage)})
	require.NoError(t, err)
	require.Len(t, medias, 1)
	require.True(t, medias[0].ExpiresAt.Equal(expiresAt.Add(time.Hour)))

	require.NoError(t, repo.DeleteByTypeAndName(ctx, provisioning.MediaTypeSeedImage, "image"))

	_, err = repo.GetByTypeAndName(ctx, provisioning.MediaTypeSeedImage, "image")
	require.ErrorIs(t, err, domain.ErrNotFound)

	medias, err = repo.GetAllWithFilter(ctx, provisioning.MediaFilter{MemberID: new(int64(1))})
	require.NoError(t, err)
	require.Len(t, medias, 1)
	require.Equal(t, provisioning.MediaTypeSecureBoot, medias[0].Type)
}
