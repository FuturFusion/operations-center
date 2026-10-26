package entities

import (
	"context"
	"time"

	"github.com/FuturFusion/operations-center/shared/api"
)

// Code generation directives.
//
//generate-database:mapper target cluster_join_token.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e cluster_join_token objects table=cluster_join_tokens
//generate-database:mapper stmt -e cluster_join_token objects-by-ServerName table=cluster_join_tokens
//generate-database:mapper stmt -e cluster_join_token id table=cluster_join_tokens
//generate-database:mapper stmt -e cluster_join_token create table=cluster_join_tokens
//generate-database:mapper stmt -e cluster_join_token update table=cluster_join_tokens
//generate-database:mapper stmt -e cluster_join_token delete-by-ServerName table=cluster_join_tokens
//
//generate-database:mapper method -e cluster_join_token GetMany table=cluster_join_tokens
//generate-database:mapper method -e cluster_join_token GetOne table=cluster_join_tokens
//generate-database:mapper method -e cluster_join_token ID table=cluster_join_tokens
//generate-database:mapper method -e cluster_join_token Exists table=cluster_join_tokens
//generate-database:mapper method -e cluster_join_token Create table=cluster_join_tokens
//generate-database:mapper method -e cluster_join_token Update table=cluster_join_tokens
//generate-database:mapper method -e cluster_join_token DeleteOne-by-ServerName table=cluster_join_tokens

type ClusterJoinToken struct {
	ID                 int64
	ServerName         string `db:"primary=yes"`
	Secret             string
	ClusterFingerprint string
	ClusterAddresses   []string `db:"marshal=json"`
	ExpirationDate     time.Time
}

type ClusterJoinTokenFilter struct {
	ID         *int64
	ServerName *string
}

type ClusterJoinTokens []ClusterJoinToken

func (ts ClusterJoinTokens) ToAPI() []api.ClusterJoinToken {
	tokens := make([]api.ClusterJoinToken, 0, len(ts))
	for _, t := range ts {
		tokens = append(tokens, t.ToAPI())
	}

	return tokens
}

func (t ClusterJoinToken) ToAPI() api.ClusterJoinToken {
	return api.ClusterJoinToken{
		ServerName:         t.ServerName,
		Secret:             t.Secret,
		ClusterFingerprint: t.ClusterFingerprint,
		ClusterAddresses:   t.ClusterAddresses,
		ExpirationDate:     t.ExpirationDate,
	}
}

func DeleteExpiredTokens(ctx context.Context, tx tx) error {
	tokens, err := GetClusterJoinTokens(ctx, tx)
	if err != nil {
		return err
	}

	for _, t := range tokens {
		if t.ExpirationDate.Before(time.Now()) {
			err = DeleteClusterJoinToken(ctx, tx, t.ServerName)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
