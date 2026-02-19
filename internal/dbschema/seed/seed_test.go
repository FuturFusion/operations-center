package seed_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/dbschema/seed"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
)

func TestSeedDB(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	db, err := dbdriver.Open(tmpDir)
	require.NoError(t, err)

	// The main performance boost originates from `synchronous = 0`
	_, err = db.ExecContext(ctx, `
PRAGMA journal_mode = OFF;
PRAGMA synchronous = 0;
PRAGMA cache_size = 1000000;
PRAGMA locking_mode = EXCLUSIVE;
PRAGMA temp_store = MEMORY;
`)
	require.NoError(t, err)

	_, err = dbschema.Ensure(ctx, db, tmpDir)
	require.NoError(t, err)

	dbWithTransaction := transaction.Enable(db)
	entities.PreparedStmts, err = entities.PrepareStmts(dbWithTransaction, false)
	require.NoError(t, err)

	err = seed.DB(ctx, db, seed.Config{
		ClustersCount:           3,
		ServersMin:              3,
		ServersMax:              5,
		ImagesMin:               3,
		ImagesMax:               5,
		InstancesMin:            3,
		InstancesMax:            5,
		NetworksMin:             3,
		NetworksMax:             5,
		NetworkACLsMin:          3,
		NetworkACLsMax:          5,
		NetworkAddressSetsMin:   3,
		NetworkAddressSetsMax:   5,
		NetworkForwardsMin:      3,
		NetworkForwardsMax:      5,
		NetworkIntegrationsMin:  3,
		NetworkIntegrationsMax:  5,
		NetworkLoadBalancersMin: 3,
		NetworkLoadBalancersMax: 5,
		NetworkPeersMin:         3,
		NetworkPeersMax:         5,
		NetworkZonesMin:         3,
		NetworkZonesMax:         5,
		ProfilesMin:             3,
		ProfilesMax:             5,
		ProjectsMin:             3,
		ProjectsMax:             5,
		StorageBucketsMin:       3,
		StorageBucketsMax:       5,
		StoragePoolsMin:         3,
		StoragePoolsMax:         5,
		StorageVolumesMin:       3,
		StorageVolumesMax:       5,
	})
	require.NoError(t, err)
}
