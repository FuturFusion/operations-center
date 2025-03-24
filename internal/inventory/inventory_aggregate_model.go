package inventory

import (
	"net/url"
	"strconv"

	incusapi "github.com/lxc/incus/v6/shared/api"
)

type InventoryAggregate struct {
	Cluster              string
	Servers              []string
	Images               map[string]map[string]incusapi.Image                               // by project_name, name
	Instances            map[string]map[string]map[string]incusapi.InstanceFull             // by project_name, name, server_name
	Networks             map[string]map[string]incusapi.Network                             // by project_name, name
	NetworkACLs          map[string]map[string]incusapi.NetworkACL                          // by project_name, name
	NetworkForwards      map[string]map[string]incusapi.NetworkForward                      // by network_name, name
	NetworkIntegrations  map[string]incusapi.NetworkIntegration                             // by name
	NetworkLoadBalancers map[string]map[string]incusapi.NetworkLoadBalancer                 // by network_name, name
	NetworkPeers         map[string]map[string]incusapi.NetworkPeer                         // by network_name, name
	NetworkZones         map[string]map[string]incusapi.NetworkZone                         // by project_name, name
	Profiles             map[string]map[string]incusapi.Profile                             // by project_name, name
	Projects             map[string]incusapi.Project                                        // by name
	StorageBuckets       map[string]map[string]map[string]map[string]incusapi.StorageBucket // by project_name, storage_pool, name, server_name
	StoragePools         map[string]incusapi.StoragePool                                    // by name
	StorageVolumes       map[string]map[string]map[string]map[string]incusapi.StorageVolume // by project_name, storage_pool, type/name, server_name
}

type InventoryAggregates []InventoryAggregate

type InventoryAggregateFilter struct {
	Kinds              []string
	Cluster            *string
	Server             *string
	ServerIncludeNull  bool
	Project            *string
	ProjectIncludeNull bool
	Expression         *string
}

func (f InventoryAggregateFilter) AppendToURLValues(query url.Values) url.Values {
	for _, kind := range f.Kinds {
		query.Add("kind", kind)
	}

	if f.Cluster != nil {
		query.Add("cluster", *f.Cluster)
	}

	if f.Server != nil {
		query.Add("project", *f.Server)
	}

	if f.ServerIncludeNull {
		query.Add("server_include_null", strconv.FormatBool(f.ServerIncludeNull))
	}

	if f.Project != nil {
		query.Add("project", *f.Project)
	}

	if f.ProjectIncludeNull {
		query.Add("project_include_null", strconv.FormatBool(f.ProjectIncludeNull))
	}

	if f.Expression != nil {
		query.Add("filter", *f.Expression)
	}

	return query
}

func (f InventoryAggregateFilter) String() string {
	return f.AppendToURLValues(url.Values{}).Encode()
}
