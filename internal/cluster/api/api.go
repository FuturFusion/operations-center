package api

import "github.com/FuturFusion/operations-center/shared/api"

// InternalClusterPostHandoverRequest is a request for the /internal/membership/:handover endpoint.
type InternalClusterPostHandoverRequest struct {
	// Address of the server whose role should be transferred.
	Address string `json:"address" yaml:"address"`
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
