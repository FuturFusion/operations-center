package seed

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

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
	NetworkAddressSetsMin   int
	NetworkAddressSetsMax   int
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

	if config.NetworkAddressSetsMin == 0 {
		config.NetworkAddressSetsMin = 1
	}

	if config.NetworkAddressSetsMax == 0 {
		config.NetworkAddressSetsMax = 5
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
	networkAddressSetRepo := inventorySqlite.NewNetworkAddressSet(db)
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
				Cluster:       &clusterName,
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
			project := inventory.Project{
				Cluster: clusterName,
				Name:    projectName,
				Object: incusapi.Project{
					Name: projectName,
					ProjectPut: incusapi.ProjectPut{
						Description: projectName + " " + gofakeit.Sentence(5),
						Config: map[string]string{
							"features.images":          strconv.FormatBool(gofakeit.Bool()),
							"features.networks":        strconv.FormatBool(gofakeit.Bool()),
							"features.networks.zones":  strconv.FormatBool(gofakeit.Bool()),
							"features.profiles":        strconv.FormatBool(gofakeit.Bool()),
							"features.storage.buckets": strconv.FormatBool(gofakeit.Bool()),
							"features.storage.volumes": strconv.FormatBool(gofakeit.Bool()),
						},
					},
				},
				LastUpdated: faker.Date(),
			}

			project.DeriveUUID()
			_, err = projectRepo.Create(ctx, project)
			if err != nil {
				return err
			}
		}

		networkCount := randBetween(config.NetworksMin, config.NetworksMax)
		networks := make([]string, 0, networkCount)
		for networkIdx := 0; networkIdx < networkCount; networkIdx++ {
			networkName := fmt.Sprintf("network-%08x-%08x", clusterIdx, networkIdx)
			projectName := faker.RandomString(projects)
			networks = append(networks, networkName)
			network := inventory.Network{
				Cluster:     clusterName,
				Name:        networkName,
				ProjectName: projectName,
				Object: incusapi.Network{
					Name: networkName,
					NetworkPut: incusapi.NetworkPut{
						Description: networkName + " " + gofakeit.Sentence(5),
						Config:      map[string]string{},
					},
					Type:      randomNetworkType(),
					Managed:   gofakeit.Bool(),
					Status:    randomStatus(),
					Locations: randomSelection(servers),
					Project:   projectName,
				},
				LastUpdated: faker.Date(),
			}

			network.DeriveUUID()
			_, err = networkRepo.Create(ctx, network)
			if err != nil {
				return err
			}
		}

		storagePoolCount := randBetween(config.StoragePoolsMin, config.StoragePoolsMax)
		storagePools := make([]string, 0, storagePoolCount)
		for storagePoolIdx := 0; storagePoolIdx < storagePoolCount; storagePoolIdx++ {
			storagePoolName := fmt.Sprintf("storagePool-%08x-%08x", clusterIdx, storagePoolIdx)
			storagePools = append(storagePools, storagePoolName)
			storagePool := inventory.StoragePool{
				Cluster: clusterName,
				Name:    storagePoolName,
				Object: incusapi.StoragePool{
					Name: storagePoolName,
					StoragePoolPut: incusapi.StoragePoolPut{
						Description: storagePoolName + " " + gofakeit.Sentence(5),
						Config:      map[string]string{},
					},
					Driver:    randomStoragePoolDriver(),
					Status:    randomStatus(),
					Locations: randomSelection(servers),
				},
				LastUpdated: faker.Date(),
			}

			storagePool.DeriveUUID()
			_, err = storagePoolRepo.Create(ctx, storagePool)
			if err != nil {
				return err
			}
		}

		imageCount := randBetween(config.ImagesMin, config.ImagesMax)
		for imageIdx := 0; imageIdx < imageCount; imageIdx++ {
			imageName := fmt.Sprintf("image-%08x-%08x", clusterIdx, imageIdx)
			projectName := faker.RandomString(projects)
			image := inventory.Image{
				Cluster:     clusterName,
				Name:        imageName,
				ProjectName: projectName,
				Object: incusapi.Image{
					UpdateSource: &incusapi.ImageSource{
						Alias:     gofakeit.Word(),
						Protocol:  "simplestreams",
						Server:    gofakeit.URL(),
						ImageType: randomType(),
					},
					Aliases: []incusapi.ImageAlias{
						{
							Name:        gofakeit.Word(),
							Description: gofakeit.LoremIpsumSentence(5),
						},
					},
					Architecture: randomArchitecture(),
					Cached:       gofakeit.Bool(),
					Filename:     "/" + strings.ReplaceAll(strings.ToLower(gofakeit.Sentence(4)), " ", "/") + gofakeit.FileExtension(),
					Fingerprint:  gofakeit.HexUint(256),
					Size:         int64(gofakeit.Uint16()),
					CreatedAt:    gofakeit.Date(),
					LastUsedAt:   gofakeit.Date(),
					UploadedAt:   gofakeit.Date(),
					ImagePut: incusapi.ImagePut{
						AutoUpdate: gofakeit.Bool(),
						Properties: map[string]string{},
						Public:     gofakeit.Bool(),
						ExpiresAt:  gofakeit.Date(),
						Profiles:   []string{},
					},
					Project: projectName,
					Type:    randomType(),
				},
				LastUpdated: faker.Date(),
			}

			image.DeriveUUID()
			_, err = imageRepo.Create(ctx, image)
			if err != nil {
				return err
			}
		}

		profileCount := randBetween(config.ProfilesMin, config.ProfilesMax)
		for profileIdx := 0; profileIdx < profileCount; profileIdx++ {
			profileName := fmt.Sprintf("profile-%08x-%08x", clusterIdx, profileIdx)
			profile := inventory.Profile{
				Cluster:     clusterName,
				Name:        profileName,
				ProjectName: faker.RandomString(projects),
				Object: incusapi.Profile{
					Name: profileName,
					ProfilePut: incusapi.ProfilePut{
						Description: profileName + " " + gofakeit.Sentence(5),
						Config:      map[string]string{},
						Devices:     map[string]map[string]string{},
					},
				},
				LastUpdated: faker.Date(),
			}

			profile.DeriveUUID()
			_, err = profileRepo.Create(ctx, profile)
			if err != nil {
				return err
			}
		}

		instanceCount := randBetween(config.InstancesMin, config.InstancesMax)
		for instanceIdx := 0; instanceIdx < instanceCount; instanceIdx++ {
			instanceName := fmt.Sprintf("instance-%08x-%08x", clusterIdx, instanceIdx)
			serverName := faker.RandomString(servers)
			projectName := faker.RandomString(projects)
			instanceState := randomInstanceState()
			instance := inventory.Instance{
				Cluster:     clusterName,
				Server:      serverName,
				Name:        instanceName,
				ProjectName: projectName,
				Object: incusapi.InstanceFull{
					Instance: incusapi.Instance{
						InstancePut: incusapi.InstancePut{
							Architecture: randomArchitecture(),
							Config:       map[string]string{},
							Description:  instanceName + " " + gofakeit.Sentence(5),
							Ephemeral:    gofakeit.Bool(),
							Devices:      map[string]map[string]string{},
							Profiles:     []string{},
							Restore:      gofakeit.RandomString([]string{"snap0", "", "", "", "", "", "", ""}),
							Stateful:     gofakeit.Bool(),
						},
						CreatedAt:       gofakeit.Date(),
						Name:            instanceName,
						LastUsedAt:      gofakeit.Date(),
						Type:            randomType(),
						ExpandedConfig:  map[string]string{},
						ExpandedDevices: map[string]map[string]string{},
						Status:          instanceState,
						StatusCode:      instanceStateCode(instanceState),
						Location:        serverName,
						Project:         projectName,
					},
					Backups: []incusapi.InstanceBackup{
						{
							Name:             fmt.Sprintf("backup%d", gofakeit.IntN(10)),
							CreatedAt:        gofakeit.Date(),
							ExpiresAt:        gofakeit.Date(),
							InstanceOnly:     gofakeit.Bool(),
							OptimizedStorage: gofakeit.Bool(),
						},
					},
					State: &incusapi.InstanceState{
						Status:     instanceState,
						StatusCode: instanceStateCode(instanceState),
						Pid:        gofakeit.Int64(),
						Disk:       map[string]incusapi.InstanceStateDisk{},
						Memory:     incusapi.InstanceStateMemory{},
						Network:    map[string]incusapi.InstanceStateNetwork{},
						Processes:  int64(gofakeit.IntN(100)),
					},
					Snapshots: []incusapi.InstanceSnapshot{
						{
							InstanceSnapshotPut: incusapi.InstanceSnapshotPut{
								ExpiresAt: gofakeit.Date(),
							},
							Architecture:    randomArchitecture(),
							Config:          map[string]string{},
							CreatedAt:       gofakeit.Date(),
							Devices:         map[string]map[string]string{},
							Ephemeral:       gofakeit.Bool(),
							ExpandedConfig:  map[string]string{},
							ExpandedDevices: map[string]map[string]string{},
							LastUsedAt:      gofakeit.Date(),
							Name:            fmt.Sprintf("snapshot%d", gofakeit.IntN(20)),
							Profiles:        []string{},
							Stateful:        gofakeit.Bool(),
							Size:            int64(gofakeit.Uint32()),
						},
					},
				},
				LastUpdated: faker.Date(),
			}

			instance.DeriveUUID()
			_, err = instanceRepo.Create(ctx, instance)
			if err != nil {
				return err
			}
		}

		networkACLCount := randBetween(config.NetworkACLsMin, config.NetworkACLsMax)
		for networkACLIdx := 0; networkACLIdx < networkACLCount; networkACLIdx++ {
			networkACLName := fmt.Sprintf("networkACL-%08x-%08x", clusterIdx, networkACLIdx)
			projectName := faker.RandomString(projects)
			networkACL := inventory.NetworkACL{
				Cluster:     clusterName,
				Name:        networkACLName,
				ProjectName: projectName,
				Object: incusapi.NetworkACL{
					NetworkACLPost: incusapi.NetworkACLPost{
						Name: networkACLName,
					},
					NetworkACLPut: incusapi.NetworkACLPut{
						Description: networkACLName + " " + gofakeit.Sentence(5),
						Egress: []incusapi.NetworkACLRule{
							{
								Action:          "deny",
								Source:          gofakeit.IPv4Address(),
								Destination:     gofakeit.IPv4Address(),
								Protocol:        "ip",
								SourcePort:      strconv.FormatInt(int64(gofakeit.Uint16()), 10),
								DestinationPort: strconv.FormatInt(int64(gofakeit.Uint16()), 10),
								ICMPType:        "8",
								ICMPCode:        "0",
								Description:     networkACLName + " acl " + gofakeit.Sentence(5),
								State:           gofakeit.RandomString([]string{"enabled", "disabled"}),
							},
						},
						Ingress: []incusapi.NetworkACLRule{
							{
								Action:          "deny",
								Source:          gofakeit.IPv4Address(),
								Destination:     gofakeit.IPv4Address(),
								Protocol:        gofakeit.RandomString([]string{"tcp", "udp"}),
								SourcePort:      strconv.FormatInt(int64(gofakeit.Uint16()), 10),
								DestinationPort: strconv.FormatInt(int64(gofakeit.Uint16()), 10),
								ICMPType:        "8",
								ICMPCode:        "0",
								Description:     networkACLName + " acl " + gofakeit.Sentence(5),
								State:           gofakeit.RandomString([]string{"enabled", "disabled"}),
							},
						},
						Config: map[string]string{},
					},
					Project: projectName,
				},
				LastUpdated: faker.Date(),
			}

			networkACL.DeriveUUID()
			_, err = networkACLRepo.Create(ctx, networkACL)
			if err != nil {
				return err
			}
		}

		networkAddressSetCount := randBetween(config.NetworkAddressSetsMin, config.NetworkAddressSetsMax)
		for networkAddressSetIdx := 0; networkAddressSetIdx < networkAddressSetCount; networkAddressSetIdx++ {
			networkAddressSetName := fmt.Sprintf("networkAddressSet-%08x-%08x", clusterIdx, networkAddressSetIdx)
			projectName := faker.RandomString(projects)
			networkAddressSet := inventory.NetworkAddressSet{
				Cluster:     clusterName,
				Name:        networkAddressSetName,
				ProjectName: projectName,
				Object: incusapi.NetworkAddressSet{
					NetworkAddressSetPost: incusapi.NetworkAddressSetPost{
						Name: networkAddressSetName,
					},
					NetworkAddressSetPut: incusapi.NetworkAddressSetPut{
						Description: networkAddressSetName + " " + gofakeit.Sentence(5),
						Addresses:   ipAddresses(randBetween(0, 10)),
						Config:      map[string]string{},
					},
					Project: projectName,
				},
				LastUpdated: faker.Date(),
			}

			networkAddressSet.DeriveUUID()
			_, err = networkAddressSetRepo.Create(ctx, networkAddressSet)
			if err != nil {
				return err
			}
		}

		networkForwardCount := randBetween(config.NetworkForwardsMin, config.NetworkForwardsMax)
		for networkForwardIdx := 0; networkForwardIdx < networkForwardCount; networkForwardIdx++ {
			networkForwardName := fmt.Sprintf("networkForward-%08x-%08x", clusterIdx, networkForwardIdx)
			networkForward := inventory.NetworkForward{
				Cluster:     clusterName,
				Name:        networkForwardName,
				NetworkName: faker.RandomString(networks),
				Object: incusapi.NetworkForward{
					NetworkForwardPut: incusapi.NetworkForwardPut{
						Description: networkForwardName + " " + gofakeit.Sentence(5),
						Config:      map[string]string{},
						Ports: []incusapi.NetworkForwardPort{
							{
								Description:   networkForwardName + " forward " + gofakeit.Sentence(5),
								Protocol:      gofakeit.RandomString([]string{"tcp", "udp"}),
								ListenPort:    strconv.FormatInt(int64(gofakeit.Uint16()), 10),
								TargetPort:    strconv.FormatInt(int64(gofakeit.Uint16()), 10),
								TargetAddress: gofakeit.IPv4Address(),
							},
						},
					},
					ListenAddress: gofakeit.IPv4Address(),
					Location:      gofakeit.RandomString(servers),
				},
				LastUpdated: faker.Date(),
			}

			networkForward.DeriveUUID()
			_, err = networkForwardRepo.Create(ctx, networkForward)
			if err != nil {
				return err
			}
		}

		networkIntegrationCount := randBetween(config.NetworkIntegrationsMin, config.NetworkIntegrationsMax)
		for networkIntegrationIdx := 0; networkIntegrationIdx < networkIntegrationCount; networkIntegrationIdx++ {
			networkIntegrationName := fmt.Sprintf("networkIntegration-%08x-%08x", clusterIdx, networkIntegrationIdx)
			networkIntegration := inventory.NetworkIntegration{
				Cluster: clusterName,
				Name:    networkIntegrationName,
				Object: incusapi.NetworkIntegration{
					NetworkIntegrationPut: incusapi.NetworkIntegrationPut{
						Description: networkIntegrationName + " " + gofakeit.Sentence(5),
						Config:      map[string]string{},
					},
					Name: networkIntegrationName,
					Type: randomNetworkType(),
				},
				LastUpdated: faker.Date(),
			}

			networkIntegration.DeriveUUID()
			_, err = networkIntegrationRepo.Create(ctx, networkIntegration)
			if err != nil {
				return err
			}
		}

		networkLoadBalancerCount := randBetween(config.NetworkLoadBalancersMin, config.NetworkLoadBalancersMax)
		for networkLoadBalancerIdx := 0; networkLoadBalancerIdx < networkLoadBalancerCount; networkLoadBalancerIdx++ {
			networkLoadBalancerName := fmt.Sprintf("networkLoadBalancer-%08x-%08x", clusterIdx, networkLoadBalancerIdx)
			networkLoadBalancer := inventory.NetworkLoadBalancer{
				Cluster:     clusterName,
				Name:        networkLoadBalancerName,
				NetworkName: faker.RandomString(networks),
				Object: incusapi.NetworkLoadBalancer{
					NetworkLoadBalancerPut: incusapi.NetworkLoadBalancerPut{
						Description: networkLoadBalancerName + " " + gofakeit.Sentence(5),
						Config:      map[string]string{},
						Backends: []incusapi.NetworkLoadBalancerBackend{
							{
								Name:          gofakeit.Word(),
								Description:   networkLoadBalancerName + " backend " + gofakeit.Sentence(5),
								TargetPort:    strconv.FormatInt(int64(gofakeit.Uint16()), 10),
								TargetAddress: gofakeit.IPv4Address(),
							},
						},
						Ports: []incusapi.NetworkLoadBalancerPort{
							{
								Description:   networkLoadBalancerName + " port " + gofakeit.Sentence(5),
								Protocol:      gofakeit.RandomString([]string{"tcp", "udp"}),
								ListenPort:    strconv.FormatInt(int64(gofakeit.Uint16()), 10),
								TargetBackend: []string{gofakeit.IPv4Address()},
							},
						},
					},
				},
				LastUpdated: faker.Date(),
			}

			networkLoadBalancer.DeriveUUID()
			_, err = networkLoadBalancerRepo.Create(ctx, networkLoadBalancer)
			if err != nil {
				return err
			}
		}

		networkPeerCount := randBetween(config.NetworkPeersMin, config.NetworkPeersMax)
		for networkPeerIdx := 0; networkPeerIdx < networkPeerCount; networkPeerIdx++ {
			networkPeerName := fmt.Sprintf("networkPeer-%08x-%08x", clusterIdx, networkPeerIdx)
			networkPeer := inventory.NetworkPeer{
				Cluster:     clusterName,
				Name:        networkPeerName,
				NetworkName: faker.RandomString(networks),
				Object: incusapi.NetworkPeer{
					NetworkPeerPut: incusapi.NetworkPeerPut{
						Description: networkPeerName + " " + gofakeit.Sentence(5),
						Config:      map[string]string{},
					},
					Name:              networkPeerName,
					TargetProject:     gofakeit.RandomString(projects),
					TargetNetwork:     gofakeit.IPv4Address(),
					Status:            randomStatus(),
					Type:              randomNetworkType(),
					TargetIntegration: "ovn-ic1",
				},
				LastUpdated: faker.Date(),
			}

			networkPeer.DeriveUUID()
			_, err = networkPeerRepo.Create(ctx, networkPeer)
			if err != nil {
				return err
			}
		}

		networkZoneCount := randBetween(config.NetworkZonesMin, config.NetworkZonesMax)
		for networkZoneIdx := 0; networkZoneIdx < networkZoneCount; networkZoneIdx++ {
			networkZoneName := fmt.Sprintf("networkZone-%08x-%08x", clusterIdx, networkZoneIdx)
			projectName := faker.RandomString(projects)
			networkZone := inventory.NetworkZone{
				Cluster:     clusterName,
				Name:        networkZoneName,
				ProjectName: projectName,
				Object: incusapi.NetworkZone{
					NetworkZonePut: incusapi.NetworkZonePut{
						Description: networkZoneName + " " + gofakeit.Sentence(5),
						Config:      map[string]string{},
					},
					Name:    networkZoneName,
					Project: projectName,
				},
				LastUpdated: faker.Date(),
			}

			networkZone.DeriveUUID()
			_, err = networkZoneRepo.Create(ctx, networkZone)
			if err != nil {
				return err
			}
		}

		storageBucketCount := randBetween(config.StorageBucketsMin, config.StorageBucketsMax)
		for storageBucketIdx := 0; storageBucketIdx < storageBucketCount; storageBucketIdx++ {
			storageBucketName := fmt.Sprintf("storageBucket-%08x-%08x", clusterIdx, storageBucketIdx)
			projectName := faker.RandomString(projects)
			storageBucket := inventory.StorageBucket{
				Cluster:         clusterName,
				Server:          faker.RandomString(servers),
				Name:            storageBucketName,
				StoragePoolName: faker.RandomString(storagePools),
				ProjectName:     projectName,
				Object: incusapi.StorageBucket{
					StorageBucketPut: incusapi.StorageBucketPut{
						Config:      map[string]string{},
						Description: storageBucketName + " " + gofakeit.Sentence(5),
					},
					Name:     storageBucketName,
					S3URL:    gofakeit.URL(),
					Location: gofakeit.RandomString(servers),
					Project:  projectName,
				},
				LastUpdated: faker.Date(),
			}

			storageBucket.DeriveUUID()
			_, err = storageBucketRepo.Create(ctx, storageBucket)
			if err != nil {
				return err
			}
		}

		storageVolumeCount := randBetween(config.StorageVolumesMin, config.StorageVolumesMax)
		for storageVolumeIdx := 0; storageVolumeIdx < storageVolumeCount; storageVolumeIdx++ {
			storageVolumeName := fmt.Sprintf("storageVolume-%08x-%08x", clusterIdx, storageVolumeIdx)
			projectName := faker.RandomString(projects)
			storageVolume := inventory.StorageVolume{
				Cluster:         clusterName,
				Server:          faker.RandomString(servers),
				Name:            storageVolumeName,
				StoragePoolName: faker.RandomString(storagePools),
				ProjectName:     projectName,
				Type:            gofakeit.RandomString([]string{"container", "virtual-machine"}),
				Object: incusapi.StorageVolume{
					StorageVolumePut: incusapi.StorageVolumePut{
						Config:      map[string]string{},
						Description: storageVolumeName + " " + gofakeit.Sentence(5),
						Restore:     gofakeit.Word(),
					},
					Name:        storageVolumeName,
					Type:        "custom",
					Location:    gofakeit.RandomString(servers),
					ContentType: gofakeit.RandomString([]string{"filesystem", "block"}),
					Project:     projectName,
					CreatedAt:   gofakeit.Date(),
				},
				LastUpdated: faker.Date(),
			}

			storageVolume.DeriveUUID()
			_, err = storageVolumeRepo.Create(ctx, storageVolume)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
