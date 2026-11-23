package api

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

type ClusterBootstrapPost struct {
	ServerName string `json:"server_name" yaml:"server_name"`

	ClusterAddress string `json:"cluster_address" yaml:"cluster_address"`
}
