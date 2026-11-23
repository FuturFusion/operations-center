package config

import (
	"context"
	"strings"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type ifMatchKey struct{}

// WithIfMatch returns ctx carrying the ETag a change of a shared config section is expected to replace.
func WithIfMatch(ctx context.Context, etag string) context.Context {
	return context.WithValue(ctx, ifMatchKey{}, strings.Trim(etag, "\""))
}

// CheckETag fails if the ETag carried by ctx doesn't match the changed section of stored.
// Without an ETag the change replaces whatever is stored.
func (c SharedChange) CheckETag(ctx context.Context, stored SharedConfig) error {
	etag, ok := ctx.Value(ifMatchKey{}).(string)
	if !ok || etag == "" {
		return nil
	}

	normalized, err := normalizeShared(stored)
	if err != nil {
		return err
	}

	hash, err := response.EtagHash(sharedSection(normalized, c.section))
	if err != nil {
		return err
	}

	if hash != etag {
		return domain.NewErrorf(domain.ErrPreconditionFailed, api.ErrorReasonPreconditionFailed, "The configuration was changed in the meantime").
			WithHintf("Fetch the configuration again and reapply the change.")
	}

	return nil
}

// sharedSection returns the section s of cfg, all of cfg for sectionChanged.
func sharedSection(cfg SharedConfig, s section) any {
	switch s {
	case sectionNetwork:
		return cfg.Network

	case sectionSecurity:
		return cfg.Security

	case sectionSettings:
		return cfg.Settings

	case sectionUpdates:
		return cfg.Updates

	case sectionCluster:
		return cfg.Cluster

	default:
		return cfg
	}
}
