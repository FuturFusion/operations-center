package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/pflag"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/dbschema/seed"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
)

func main() {
	ctx := context.Background()

	flagLogDebug := pflag.BoolP("debug", "d", false, "Show all debug messages")
	flagLogVerbose := pflag.BoolP("verbose", "v", false, "Show all information messages")
	flagDBDir := pflag.String("db-dir", "./", "directory path to store the local.db. Directory is created, if it does not exist.")
	flagDBForceOverwrite := pflag.BoolP("force", "f", false, "if force flag is provided, an existing DB is overwritten")

	flagClustersCount := pflag.Int("clusters", 2, "number of clusters to create")
	flagServersMin := pflag.Int("servers-min", 1, "min servers to create per cluster")
	flagServersMax := pflag.Int("servers-max", 10, "max servers to create per cluster")
	flagProjectsMin := pflag.Int("projects-min", 2, "min projects to create per cluster")
	flagProjectsMax := pflag.Int("projects-max", 5, "max projects to create per cluster")
	flagImagesMin := pflag.Int("images-min", 2, "min images to create per cluster")
	flagImagesMax := pflag.Int("images-max", 5, "max images to create per cluster")
	flagInstancesMin := pflag.Int("instance-min", 10, "min instance to create per cluster")
	flagInstancesMax := pflag.Int("instance-max", 20, "max instance to create per cluster")
	flagNetworksMin := pflag.Int("networks-min", 1, "min networks to create per cluster")
	flagNetworksMax := pflag.Int("networks-max", 10, "max networks to create per cluster")
	flagNetworkACLsMin := pflag.Int("network-acls-min", 1, "min network acls to create per cluster")
	flagNetworkACLsMax := pflag.Int("network-acls-max", 5, "max network acls to create per cluster")
	flagNetworkAddressSetsMin := pflag.Int("network-address-sets-min", 1, "min network address-sets to create per cluster")
	flagNetworkAddressSetsMax := pflag.Int("network-address-sets-max", 5, "max network address-sets to create per cluster")
	flagNetworkForwardsMin := pflag.Int("network-forwards-min", 1, "min network forwards to create per cluster")
	flagNetworkForwardsMax := pflag.Int("network-forwards-max", 5, "max network forwards to create per cluster")
	flagNetworkIntegrationsMin := pflag.Int("network-integrations-min", 1, "min network integrations to create per cluster")
	flagNetworkIntegrationsMax := pflag.Int("network-integrations-max", 5, "max network integrations to create per cluster")
	flagNetworkLoadBalancersMin := pflag.Int("network-load-balancers-min", 1, "min network load-balancers to create per cluster")
	flagNetworkLoadBalancersMax := pflag.Int("network-load-balancers-max", 5, "max network load-balancers to create per cluster")
	flagNetworkPeersMin := pflag.Int("network-peers-min", 1, "min network peers to create per cluster")
	flagNetworkPeersMax := pflag.Int("network-peers-max", 5, "max network peers to create per cluster")
	flagNetworkZonesMin := pflag.Int("network-zones-min", 1, "min network zones to create per cluster")
	flagNetworkZonesMax := pflag.Int("network-zones-max", 5, "max network zones to create per cluster")
	flagProfilesMin := pflag.Int("profiles-min", 1, "min profiles to create per cluster")
	flagProfilesMax := pflag.Int("profiles-max", 5, "max profiles to create per cluster")
	flagStorageBucketsMin := pflag.Int("storage-buckets-min", 1, "min storage-buckets to create per cluster")
	flagStorageBucketsMax := pflag.Int("storage-buckets-max", 5, "max storage-buckets to create per cluster")
	flagStoragePoolsMin := pflag.Int("storage-pools-min", 1, "min storage pools to create per cluster")
	flagStoragePoolsMax := pflag.Int("storage-pools-max", 5, "max storage pools to create per cluster")
	flagStorageVolumesMin := pflag.Int("storage-volumes-min", 1, "min storage volumes to create per cluster")
	flagStorageVolumesMax := pflag.Int("storage-volumes-max", 5, "max storage volumes to create per cluster")

	pflag.Parse()

	err := logger.InitLogger(os.Stderr, "", *flagLogVerbose, *flagLogDebug)
	die(err)

	err = os.MkdirAll(*flagDBDir, 0o700)
	die(err)

	dbFilename := filepath.Join(*flagDBDir, "local.db")
	_, err = os.Stat(dbFilename)
	if err == nil {
		if !*flagDBForceOverwrite {
			slog.ErrorContext(ctx, "DB file does already exist and --force is not provided", slog.String("db", dbFilename))
			os.Exit(1)
		}

		err = os.Remove(dbFilename)
		die(err)
	}

	if err != nil && !os.IsNotExist(err) {
		die(err)
	}

	db, err := dbdriver.Open(*flagDBDir)
	die(err)

	// The main performance boost originates from `synchronous = 0`
	_, err = db.ExecContext(ctx, `
PRAGMA journal_mode = OFF;
PRAGMA synchronous = 0;
PRAGMA cache_size = 1000000;
PRAGMA locking_mode = EXCLUSIVE;
PRAGMA temp_store = MEMORY;
`)
	die(err)

	_, err = dbschema.Ensure(ctx, db, *flagDBDir)
	die(err)

	dbWithTransaction := transaction.Enable(db)
	entities.PreparedStmts, err = entities.PrepareStmts(dbWithTransaction, false)
	die(err)

	err = seed.DB(ctx, db, seed.Config{
		ClustersCount:           *flagClustersCount,
		ServersMin:              *flagServersMin,
		ServersMax:              *flagServersMax,
		ProjectsMin:             *flagProjectsMin,
		ProjectsMax:             *flagProjectsMax,
		ImagesMin:               *flagImagesMin,
		ImagesMax:               *flagImagesMax,
		InstancesMin:            *flagInstancesMin,
		InstancesMax:            *flagInstancesMax,
		NetworksMin:             *flagNetworksMin,
		NetworksMax:             *flagNetworksMax,
		NetworkACLsMin:          *flagNetworkACLsMin,
		NetworkACLsMax:          *flagNetworkACLsMax,
		NetworkAddressSetsMin:   *flagNetworkAddressSetsMin,
		NetworkAddressSetsMax:   *flagNetworkAddressSetsMax,
		NetworkForwardsMin:      *flagNetworkForwardsMin,
		NetworkForwardsMax:      *flagNetworkForwardsMax,
		NetworkIntegrationsMin:  *flagNetworkIntegrationsMin,
		NetworkIntegrationsMax:  *flagNetworkIntegrationsMax,
		NetworkLoadBalancersMin: *flagNetworkLoadBalancersMin,
		NetworkLoadBalancersMax: *flagNetworkLoadBalancersMax,
		NetworkPeersMin:         *flagNetworkPeersMin,
		NetworkPeersMax:         *flagNetworkPeersMax,
		NetworkZonesMin:         *flagNetworkZonesMin,
		NetworkZonesMax:         *flagNetworkZonesMax,
		ProfilesMin:             *flagProfilesMin,
		ProfilesMax:             *flagProfilesMax,
		StorageBucketsMin:       *flagStorageBucketsMin,
		StorageBucketsMax:       *flagStorageBucketsMax,
		StoragePoolsMin:         *flagStoragePoolsMin,
		StoragePoolsMax:         *flagStoragePoolsMax,
		StorageVolumesMin:       *flagStorageVolumesMin,
		StorageVolumesMax:       *flagStorageVolumesMax,
	})
	die(err)
}

// die is a convenience function to end the processing with a panic in the case of an error.
func die(err error) {
	if err != nil {
		slog.ErrorContext(context.Background(), "generate-inventory failed", slog.Any("err", err))
		panic("die")
	}
}
