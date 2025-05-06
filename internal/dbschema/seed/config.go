package seed

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
