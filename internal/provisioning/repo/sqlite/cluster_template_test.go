package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestClusterTemplateDatabaseActions(t *testing.T) {
	clusterTemplateA := provisioning.ClusterTemplate{
		Name:                  "A",
		Description:           "A",
		ServiceConfigTemplate: `{}`,
		ApplicationConfigTemplate: `{
  "key": "@BAR@"
}`,
		Variables: api.ClusterTemplateVariables{
			"FOO": api.ClusterTemplateVariable{
				Description:  "foo",
				DefaultValue: "1",
			},
		},
	}

	clusterTemplateB := provisioning.ClusterTemplate{
		Name:        "B",
		Description: "B",
		ServiceConfigTemplate: `{
  "key": "@BAR@"
}`,
		ApplicationConfigTemplate: `{}`,
		Variables: api.ClusterTemplateVariables{
			"BAR": api.ClusterTemplateVariable{
				Description: "BAR",
			},
		},
	}

	ctx := context.Background()

	// Create a new temporary database.
	tmpDir := t.TempDir()
	db, err := dbdriver.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	_, err = dbschema.Ensure(ctx, db, tmpDir)
	require.NoError(t, err)

	tx := transaction.Enable(db)
	entities.PreparedStmts, err = entities.PrepareStmts(tx, false)
	require.NoError(t, err)

	clusterTemplate := sqlite.NewClusterTemplate(tx)

	// Add cluster templates
	_, err = clusterTemplate.Create(ctx, clusterTemplateA)
	require.NoError(t, err)
	_, err = clusterTemplate.Create(ctx, clusterTemplateB)
	require.NoError(t, err)

	// Ensure we have two entries
	clusters, err := clusterTemplate.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, clusters, 2)

	clusterIDs, err := clusterTemplate.GetAllNames(ctx)
	require.NoError(t, err)
	require.Len(t, clusterIDs, 2)
	require.ElementsMatch(t, []string{"A", "B"}, clusterIDs)

	// Should get back clusterTemplateA unchanged.
	dbClusterA, err := clusterTemplate.GetByName(ctx, clusterTemplateA.Name)
	require.NoError(t, err)
	clusterTemplateA.ID = dbClusterA.ID
	clusterTemplateA.LastUpdated = dbClusterA.LastUpdated
	require.Equal(t, clusterTemplateA, *dbClusterA)

	dbClusterB, err := clusterTemplate.GetByName(ctx, clusterTemplateB.Name)
	require.NoError(t, err)
	clusterTemplateB.ID = dbClusterB.ID
	clusterTemplateB.LastUpdated = dbClusterB.LastUpdated
	require.Equal(t, clusterTemplateB, *dbClusterB)

	// Test updating a cluster template.
	clusterTemplateB.Description = "updated"
	err = clusterTemplate.Update(ctx, clusterTemplateB)
	require.NoError(t, err)
	clusterTemplateB.Name = "B new"
	err = clusterTemplate.Rename(ctx, "B", clusterTemplateB.Name)
	require.NoError(t, err)
	dbClusterB, err = clusterTemplate.GetByName(ctx, clusterTemplateB.Name)
	require.NoError(t, err)
	clusterTemplateB.ID = dbClusterB.ID
	clusterTemplateB.LastUpdated = dbClusterB.LastUpdated
	require.Equal(t, clusterTemplateB, *dbClusterB)

	// Delete a cluster template.
	err = clusterTemplate.DeleteByName(ctx, clusterTemplateA.Name)
	require.NoError(t, err)
	_, err = clusterTemplate.GetByName(ctx, clusterTemplateA.Name)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Should have one cluster templates remaining.
	clusters, err = clusterTemplate.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, clusters, 1)

	// Can't delete a cluster template that doesn't exist.
	err = clusterTemplate.DeleteByName(ctx, "three")
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update a cluster template that doesn't exist.
	err = clusterTemplate.Update(ctx, clusterTemplateA)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't add a duplicate cluster template.
	_, err = clusterTemplate.Create(ctx, clusterTemplateB)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)
}
