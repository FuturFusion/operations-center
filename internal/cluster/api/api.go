package api

import "github.com/FuturFusion/operations-center/shared/api"

// InternalRaftNode represents a node that is part of the cowsql raft cluster.
type InternalRaftNode struct {
	ID      uint64 `json:"id" yaml:"id"`
	Address string `json:"address" yaml:"address"`
	Role    int    `json:"role" yaml:"role"`
	Name    string `json:"name" yaml:"name"`
}
