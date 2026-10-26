package api

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

type ClusterMember struct {
	Name          string    `json:"name" yaml:"name"`
	Address       string    `json:"address" yaml:"address"`
	Roles         []string  `json:"roles" yaml:"roles"`
	LastHeartbeat time.Time `json:"last_heartbeat" yaml:"last_heartbeat"`
	Leader        bool      `json:"leader" yaml:"leader"`

	// Offline is true if the member missed its heartbeats for longer than the offline threshold.
	Offline bool `json:"offline" yaml:"offline"`
}

type ClusterBootstrapPost struct {
	ServerName string `json:"server_name" yaml:"server_name"`

	ClusterAddress string `json:"cluster_address" yaml:"cluster_address"`
}

type ClusterJoinPost struct {
	ServerName string `json:"server_name" yaml:"server_name"`

	Token string `json:"token" yaml:"token"`

	ClusterAddress string `json:"cluster_address" yaml:"cluster_address"`
}

type ClusterTokenPost struct {
	ServerName string `json:"server_name" yaml:"server_name"`

	Expiry string `json:"expiry" yaml:"expiry"`
}

type ClusterJoinToken struct {
	ServerName         string    `json:"server_name" yaml:"server_name"`
	Secret             string    `json:"secret" yaml:"secret"`
	ClusterFingerprint string    `json:"cluster_fingerprint" yaml:"cluster_fingerprint"`
	ClusterAddresses   []string  `json:"cluster_addresses" yaml:"cluster_addresses"`
	ExpirationDate     time.Time `json:"expiration_date" yaml:"expiration_date"`
}

type ClusterAcceptPost struct {
	ServerName    string      `json:"server_name" yaml:"server_name"`
	Address       string      `json:"address" yaml:"address"`
	SchemaVersion int         `json:"schema_version" yaml:"schema_version"`
	APIVersion    int         `json:"api_version" yaml:"api_version"`
	Architecture  int         `json:"architecture" yaml:"architecture"`
	Certificate   Certificate `json:"certificate" yaml:"certificate"`
}

type ServerCertificate struct {
	ServerName  string      `json:"server_name" yaml:"server_name"`
	Certificate Certificate `json:"certificate" yaml:"certificate"`
}

func (c ClusterJoinToken) String() (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func DecodeJoinToken(token string) (*ClusterJoinToken, error) {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}

	var obj ClusterJoinToken
	err = json.Unmarshal(b, &obj)
	if err != nil {
		return nil, err
	}

	return &obj, nil
}

type ClusterTrustPost struct {
	CandidateServerCertificate string `json:"candidate_server_certificate" yaml:"candidate_server_certificate"`
	ClusterJoinToken           string `json:"cluster_join_token" yaml:"cluster_join_token"`
}
