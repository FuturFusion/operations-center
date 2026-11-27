package api

import "github.com/FuturFusion/operations-center/shared/api"

// InternalClusterPostHandoverRequest is a request for the /internal/membership/:handover endpoint.
type InternalClusterPostHandoverRequest struct {
	// Address of the server whose role should be transferred.
	Address string `json:"address" yaml:"address"`
}

// InternalClusterPostTrustRequest is a request for the /internal/membership/:trust endpoint.
type InternalClusterPostTrustRequest struct {
	// Token is the join token of the joining server.
	Token string `json:"token" yaml:"token"`
}

// InternalRaftNode represents a node that is part of the cowsql raft cluster.
type InternalRaftNode struct {
	ID      uint64 `json:"id" yaml:"id"`
	Address string `json:"address" yaml:"address"`
	Role    int    `json:"role" yaml:"role"`
	Name    string `json:"name" yaml:"name"`
}

// InternalClusterPostAssignRequest is a request for the /internal/membership/:assign endpoint.
type InternalClusterPostAssignRequest struct {
	RaftNodes []InternalRaftNode `json:"raft_nodes" yaml:"raft_nodes"`
}

type InternalClusterAcceptResponse struct {
	LeaderAddress      string                  `json:"leader_address" yaml:"leader_address"`
	RaftNodes          []InternalRaftNode      `json:"raft_nodes" yaml:"raft_nodes"`
	PublicKey          string                  `json:"public_key" yaml:"public_key"`
	PrivateKey         string                  `json:"private_key" yaml:"private_key"`
	ServerCertificates []api.ServerCertificate `json:"server_certificates" yaml:"server_certificates"`

	// ClientCertificate and ClientKey are the client keypair shared by the cluster members.
	ClientCertificate string `json:"client_certificate" yaml:"client_certificate"`
	ClientKey         string `json:"client_key" yaml:"client_key"`
}

// States of a leader task on a cluster member.
const (
	// TaskRunning is a leader task started on the member.
	TaskRunning = "running"

	// TaskStopping is a leader task stopped on the member, while some of its work is still in flight.
	TaskStopping = "stopping"

	// TaskStopped is a leader task neither started nor working on the member.
	TaskStopped = "stopped"
)

// InternalClusterTask is the state of a leader task on a cluster member.
type InternalClusterTask struct {
	Name  string `json:"name" yaml:"name"`
	State string `json:"state" yaml:"state"`
}

// InternalClusterTaskStatus is a response of the /internal/membership/tasks/:status endpoint.
type InternalClusterTaskStatus struct {
	// Address is the cluster address of the member.
	Address string `json:"address" yaml:"address"`

	Tasks []InternalClusterTask `json:"tasks" yaml:"tasks"`
}
