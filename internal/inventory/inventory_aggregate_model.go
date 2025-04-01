package inventory

import (
	"net/url"
	"strconv"
)

type InventoryAggregate struct {
	Cluster              string
	Servers              []string
	Images               Images
	Instances            Instances
	Networks             Networks
	NetworkACLs          NetworkACLs
	NetworkForwards      NetworkForwards
	NetworkIntegrations  NetworkIntegrations
	NetworkLoadBalancers NetworkLoadBalancers
	NetworkPeers         NetworkPeers
	NetworkZones         NetworkZones
	Profiles             Profiles
	Projects             Projects
	StorageBuckets       StorageBuckets
	StoragePools         StoragePools
	StorageVolumes       StorageVolumes
}

type InventoryAggregates []InventoryAggregate

type InventoryAggregateFilter struct {
	Kinds              []string
	Clusters           []string
	Servers            []string
	ServerIncludeNull  bool
	Projects           []string
	ProjectIncludeNull bool
	Parents            []string
	ParentIncludeNull  bool
	Expression         *string
}

func (f InventoryAggregateFilter) AppendToURLValues(query url.Values) url.Values {
	for _, kind := range f.Kinds {
		query.Add("kind", kind)
	}

	for _, cluster := range f.Clusters {
		query.Add("cluster", cluster)
	}

	for _, server := range f.Servers {
		query.Add("server", server)
	}

	if f.ServerIncludeNull {
		query.Add("server_include_null", strconv.FormatBool(f.ServerIncludeNull))
	}

	for _, project := range f.Projects {
		query.Add("project", project)
	}

	if f.ProjectIncludeNull {
		query.Add("project_include_null", strconv.FormatBool(f.ProjectIncludeNull))
	}

	for _, parent := range f.Parents {
		query.Add("parent", parent)
	}

	if f.ParentIncludeNull {
		query.Add("parent_include_null", strconv.FormatBool(f.ParentIncludeNull))
	}

	if f.Expression != nil {
		query.Add("filter", *f.Expression)
	}

	return query
}

func (f InventoryAggregateFilter) String() string {
	return f.AppendToURLValues(url.Values{}).Encode()
}
