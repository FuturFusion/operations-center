package seed

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"

	"github.com/brianvoe/gofakeit/v7"
	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/inventory"
	inventorySqlite "github.com/FuturFusion/operations-center/internal/inventory/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	provisioningSqlite "github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/shared/api"
)

type Config struct {
	ClustersCount           int
	ServersMin              int
	ServersMax              int
	ProjectsMin             int
	ProjectsMax             int
	ImagesMin               int
	ImagesMax               int
	InstancesMin            int
	InstancesMax            int
	NetworksMin             int
	NetworksMax             int
	NetworkACLsMin          int
	NetworkACLsMax          int
	NetworkForwardsMin      int
	NetworkForwardsMax      int
	NetworkIntegrationsMin  int
	NetworkIntegrationsMax  int
	NetworkLoadBalancersMin int
	NetworkLoadBalancersMax int
	NetworkPeersMin         int
	NetworkPeersMax         int
	NetworkZonesMin         int
	NetworkZonesMax         int
	ProfilesMin             int
	ProfilesMax             int
	StorageBucketsMin       int
	StorageBucketsMax       int
	StoragePoolsMin         int
	StoragePoolsMax         int
	StorageVolumesMin       int
	StorageVolumesMax       int
}

func configDefaults(config Config) Config {
	if config.ClustersCount == 0 {
		config.ClustersCount = 2
	}

	if config.ServersMin == 0 {
		config.ServersMin = 1
	}

	if config.ServersMax == 0 {
		config.ServersMax = 10
	}

	if config.ProjectsMin == 0 {
		config.ProjectsMin = 2
	}

	if config.ProjectsMax == 0 {
		config.ProjectsMax = 5
	}

	if config.ImagesMin == 0 {
		config.ImagesMin = 2
	}

	if config.ImagesMax == 0 {
		config.ImagesMax = 5
	}

	if config.InstancesMin == 0 {
		config.InstancesMin = 10
	}

	if config.InstancesMax == 0 {
		config.InstancesMax = 20
	}

	if config.NetworksMin == 0 {
		config.NetworksMin = 1
	}

	if config.NetworksMax == 0 {
		config.NetworksMax = 10
	}

	if config.NetworkACLsMin == 0 {
		config.NetworkACLsMin = 1
	}

	if config.NetworkACLsMax == 0 {
		config.NetworkACLsMax = 5
	}

	if config.NetworkForwardsMin == 0 {
		config.NetworkForwardsMin = 1
	}

	if config.NetworkForwardsMax == 0 {
		config.NetworkForwardsMax = 5
	}

	if config.NetworkIntegrationsMin == 0 {
		config.NetworkIntegrationsMin = 1
	}

	if config.NetworkIntegrationsMax == 0 {
		config.NetworkIntegrationsMax = 5
	}

	if config.NetworkLoadBalancersMin == 0 {
		config.NetworkLoadBalancersMin = 1
	}

	if config.NetworkLoadBalancersMax == 0 {
		config.NetworkLoadBalancersMax = 5
	}

	if config.NetworkPeersMin == 0 {
		config.NetworkPeersMin = 1
	}

	if config.NetworkPeersMax == 0 {
		config.NetworkPeersMax = 5
	}

	if config.NetworkZonesMin == 0 {
		config.NetworkZonesMin = 1
	}

	if config.NetworkZonesMax == 0 {
		config.NetworkZonesMax = 5
	}

	if config.ProfilesMin == 0 {
		config.ProfilesMin = 1
	}

	if config.ProfilesMax == 0 {
		config.ProfilesMax = 5
	}

	if config.StorageBucketsMin == 0 {
		config.StorageBucketsMin = 1
	}

	if config.StorageBucketsMax == 0 {
		config.StorageBucketsMax = 5
	}

	if config.StoragePoolsMin == 0 {
		config.StoragePoolsMin = 1
	}

	if config.StoragePoolsMax == 0 {
		config.StoragePoolsMax = 5
	}

	if config.StorageVolumesMin == 0 {
		config.StorageVolumesMin = 1
	}

	if config.StorageVolumesMax == 0 {
		config.StorageVolumesMax = 5
	}

	return config
}

func DB(ctx context.Context, db *sql.DB, config Config) error {
	var err error

	config = configDefaults(config)

	clusterRepo := provisioningSqlite.NewCluster(db)
	serverRepo := provisioningSqlite.NewServer(db)
	projectRepo := inventorySqlite.NewProject(db)
	networkRepo := inventorySqlite.NewNetwork(db)
	storagePoolRepo := inventorySqlite.NewStoragePool(db)
	imageRepo := inventorySqlite.NewImage(db)
	profileRepo := inventorySqlite.NewProfile(db)
	instanceRepo := inventorySqlite.NewInstance(db)
	networkACLRepo := inventorySqlite.NewNetworkACL(db)
	networkForwardRepo := inventorySqlite.NewNetworkForward(db)
	networkIntegrationRepo := inventorySqlite.NewNetworkIntegration(db)
	networkLoadBalancerRepo := inventorySqlite.NewNetworkLoadBalancer(db)
	networkPeerRepo := inventorySqlite.NewNetworkPeer(db)
	networkZoneRepo := inventorySqlite.NewNetworkZone(db)
	storageBucketRepo := inventorySqlite.NewStorageBucket(db)
	storageVolumeRepo := inventorySqlite.NewStorageVolume(db)

	faker := gofakeit.New(1)
	config = configDefaults(config)

	// Populate starting from Clusters
	for clusterIdx := range config.ClustersCount {
		clusterName := fmt.Sprintf("cluster-%08x", clusterIdx)
		_, err = clusterRepo.Create(ctx, provisioning.Cluster{
			Name:          clusterName,
			ConnectionURL: fmt.Sprintf("https://%s.domain.tdl", clusterName),
			LastUpdated:   faker.Date(),
		})
		if err != nil {
			return err
		}

		serverCount := randBetween(config.ServersMin, config.ServersMax)
		servers := make([]string, 0, serverCount)
		for serverIdx := 0; serverIdx < serverCount; serverIdx++ {
			serverName := fmt.Sprintf("server-%08x-%08x", clusterIdx, serverIdx)
			servers = append(servers, serverName)
			_, err = serverRepo.Create(ctx, provisioning.Server{
				Cluster:       clusterName,
				Name:          serverName,
				Type:          api.ServerType(faker.RandomString([]string{"unknown", "incus", "migration-manager", "operations-center"})),
				ConnectionURL: fmt.Sprintf("https://%s.domain.tdl", serverName),
				LastUpdated:   faker.Date(),
			})
			if err != nil {
				return err
			}
		}

		projectCount := randBetween(config.ProjectsMin, config.ProjectsMax)
		projects := make([]string, 0, projectCount)
		for projectIdx := 0; projectIdx < projectCount; projectIdx++ {
			projectName := fmt.Sprintf("project-%08x-%08x", clusterIdx, projectIdx)
			projects = append(projects, projectName)
			_, err = projectRepo.Create(ctx, inventory.Project{
				Cluster:     clusterName,
				Name:        projectName,
				Object:      incusapi.Project{},
				LastUpdated: faker.Date(),
			})
			if err != nil {
				return err
			}
		}

		networkCount := randBetween(config.NetworksMin, config.NetworksMax)
		networks := make([]string, 0, networkCount)
		for networkIdx := 0; networkIdx < networkCount; networkIdx++ {
			networkName := fmt.Sprintf("network-%08x-%08x", clusterIdx, networkIdx)
			networks = append(networks, networkName)
			_, err = networkRepo.Create(ctx, inventory.Network{
				Cluster:     clusterName,
				Name:        networkName,
				ProjectName: faker.RandomString(projects),
				Object:      incusapi.Network{},
				LastUpdated: faker.Date(),
			})
			if err != nil {
				return err
			}
		}

		storagePoolCount := randBetween(config.StoragePoolsMin, config.StoragePoolsMax)
		storagePools := make([]string, 0, storagePoolCount)
		for storagePoolIdx := 0; storagePoolIdx < storagePoolCount; storagePoolIdx++ {
			storagePoolName := fmt.Sprintf("storagePool-%08x-%08x", clusterIdx, storagePoolIdx)
			storagePools = append(storagePools, storagePoolName)
			_, err = storagePoolRepo.Create(ctx, inventory.StoragePool{
				Cluster:     clusterName,
				Name:        storagePoolName,
				Object:      incusapi.StoragePool{},
				LastUpdated: faker.Date(),
			})
			if err != nil {
				return err
			}
		}

		imageCount := randBetween(config.ImagesMin, config.ImagesMax)
		for imageIdx := 0; imageIdx < imageCount; imageIdx++ {
			imageName := fmt.Sprintf("image-%08x-%08x", clusterIdx, imageIdx)
			_, err = imageRepo.Create(ctx, inventory.Image{
				Cluster:     clusterName,
				Name:        imageName,
				ProjectName: faker.RandomString(projects),
				Object:      incusapi.Image{},
				LastUpdated: faker.Date(),
			})
			if err != nil {
				return err
			}
		}

		profileCount := randBetween(config.ProfilesMin, config.ProfilesMax)
		for profileIdx := 0; profileIdx < profileCount; profileIdx++ {
			profileName := fmt.Sprintf("profile-%08x-%08x", clusterIdx, profileIdx)
			_, err = profileRepo.Create(ctx, inventory.Profile{
				Cluster:     clusterName,
				Name:        profileName,
				ProjectName: faker.RandomString(projects),
				Object:      incusapi.Profile{},
				LastUpdated: faker.Date(),
			})
			if err != nil {
				return err
			}
		}

		instanceCount := randBetween(config.InstancesMin, config.InstancesMax)
		for instanceIdx := 0; instanceIdx < instanceCount; instanceIdx++ {
			instanceName := fmt.Sprintf("instance-%08x-%08x", clusterIdx, instanceIdx)
			_, err = instanceRepo.Create(ctx, inventory.Instance{
				Cluster:     clusterName,
				Server:      faker.RandomString(servers),
				Name:        instanceName,
				ProjectName: faker.RandomString(projects),
				Object:      incusapi.InstanceFull{},
				LastUpdated: faker.Date(),
			})
			if err != nil {
				return err
			}
		}

		networkACLCount := randBetween(config.NetworkACLsMin, config.NetworkACLsMax)
		for networkACLIdx := 0; networkACLIdx < networkACLCount; networkACLIdx++ {
			networkACLName := fmt.Sprintf("networkACL-%08x-%08x", clusterIdx, networkACLIdx)
			_, err = networkACLRepo.Create(ctx, inventory.NetworkACL{
				Cluster:     clusterName,
				Name:        networkACLName,
				ProjectName: faker.RandomString(projects),
				Object:      incusapi.NetworkACL{},
				LastUpdated: faker.Date(),
			})
			if err != nil {
				return err
			}
		}

		networkForwardCount := randBetween(config.NetworkForwardsMin, config.NetworkForwardsMax)
		for networkForwardIdx := 0; networkForwardIdx < networkForwardCount; networkForwardIdx++ {
			networkForwardName := fmt.Sprintf("networkForward-%08x-%08x", clusterIdx, networkForwardIdx)
			_, err = networkForwardRepo.Create(ctx, inventory.NetworkForward{
				Cluster:     clusterName,
				Name:        networkForwardName,
				NetworkName: faker.RandomString(networks),
				Object:      incusapi.NetworkForward{},
				LastUpdated: faker.Date(),
			})
			if err != nil {
				return err
			}
		}

		networkIntegrationCount := randBetween(config.NetworkIntegrationsMin, config.NetworkIntegrationsMax)
		for networkIntegrationIdx := 0; networkIntegrationIdx < networkIntegrationCount; networkIntegrationIdx++ {
			networkIntegrationName := fmt.Sprintf("networkIntegration-%08x-%08x", clusterIdx, networkIntegrationIdx)
			_, err = networkIntegrationRepo.Create(ctx, inventory.NetworkIntegration{
				Cluster:     clusterName,
				Name:        networkIntegrationName,
				Object:      incusapi.NetworkIntegration{},
				LastUpdated: faker.Date(),
			})
			if err != nil {
				return err
			}
		}

		networkLoadBalancerCount := randBetween(config.NetworkLoadBalancersMin, config.NetworkLoadBalancersMax)
		for networkLoadBalancerIdx := 0; networkLoadBalancerIdx < networkLoadBalancerCount; networkLoadBalancerIdx++ {
			networkLoadBalancerName := fmt.Sprintf("networkLoadBalancer-%08x-%08x", clusterIdx, networkLoadBalancerIdx)
			_, err = networkLoadBalancerRepo.Create(ctx, inventory.NetworkLoadBalancer{
				Cluster:     clusterName,
				Name:        networkLoadBalancerName,
				NetworkName: faker.RandomString(networks),
				Object:      incusapi.NetworkLoadBalancer{},
				LastUpdated: faker.Date(),
			})
			if err != nil {
				return err
			}
		}

		networkPeerCount := randBetween(config.NetworkPeersMin, config.NetworkPeersMax)
		for networkPeerIdx := 0; networkPeerIdx < networkPeerCount; networkPeerIdx++ {
			networkPeerName := fmt.Sprintf("networkPeer-%08x-%08x", clusterIdx, networkPeerIdx)
			_, err = networkPeerRepo.Create(ctx, inventory.NetworkPeer{
				Cluster:     clusterName,
				Name:        networkPeerName,
				NetworkName: faker.RandomString(networks),
				Object:      incusapi.NetworkPeer{},
				LastUpdated: faker.Date(),
			})
			if err != nil {
				return err
			}
		}

		networkZoneCount := randBetween(config.NetworkZonesMin, config.NetworkZonesMax)
		for networkZoneIdx := 0; networkZoneIdx < networkZoneCount; networkZoneIdx++ {
			networkZoneName := fmt.Sprintf("networkZone-%08x-%08x", clusterIdx, networkZoneIdx)
			_, err = networkZoneRepo.Create(ctx, inventory.NetworkZone{
				Cluster:     clusterName,
				Name:        networkZoneName,
				ProjectName: faker.RandomString(projects),
				Object:      incusapi.NetworkZone{},
				LastUpdated: faker.Date(),
			})
			if err != nil {
				return err
			}
		}

		storageBucketCount := randBetween(config.StorageBucketsMin, config.StorageBucketsMax)
		for storageBucketIdx := 0; storageBucketIdx < storageBucketCount; storageBucketIdx++ {
			storageBucketName := fmt.Sprintf("storageBucket-%08x-%08x", clusterIdx, storageBucketIdx)
			_, err = storageBucketRepo.Create(ctx, inventory.StorageBucket{
				Cluster:         clusterName,
				Server:          faker.RandomString(servers),
				Name:            storageBucketName,
				StoragePoolName: faker.RandomString(storagePools),
				ProjectName:     faker.RandomString(projects),
				Object:          incusapi.StorageBucket{},
				LastUpdated:     faker.Date(),
			})
			if err != nil {
				return err
			}
		}

		storageVolumeCount := randBetween(config.StorageVolumesMin, config.StorageVolumesMax)
		for storageVolumeIdx := 0; storageVolumeIdx < storageVolumeCount; storageVolumeIdx++ {
			storageVolumeName := fmt.Sprintf("storageVolume-%08x-%08x", clusterIdx, storageVolumeIdx)
			_, err = storageVolumeRepo.Create(ctx, inventory.StorageVolume{
				Cluster:         clusterName,
				Server:          faker.RandomString(servers),
				Name:            storageVolumeName,
				StoragePoolName: faker.RandomString(storagePools),
				ProjectName:     faker.RandomString(projects),
				Type:            gofakeit.RandomString([]string{"container", "virtual-machine"}),
				Object:          incusapi.StorageVolume{},
				LastUpdated:     faker.Date(),
			})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func randBetween(from, to int) int {
	if from == to {
		return from
	}

	if to < from {
		to, from = from, to
	}

	if from < 0 {
		from = 0
	}

	if to < 0 {
		to = 10
	}

	return rand.IntN(to-from) + from
}
