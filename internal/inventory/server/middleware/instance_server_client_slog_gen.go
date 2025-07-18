// Code generated by mockery. DO NOT EDIT.
// template: github.com/FuturFusion/operations-center/internal/logger/slog.gotmpl

package middleware

import (
	"context"
	"log/slog"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/lxc/incus/v6/shared/api"
)

// InstanceServerClientWithSlog implements inventory.InstanceServerClient that is instrumented with slog logger.
type InstanceServerClientWithSlog struct {
	_log                  *slog.Logger
	_base                 inventory.InstanceServerClient
	_isInformativeErrFunc func(error) bool
}

type InstanceServerClientWithSlogOption func(s *InstanceServerClientWithSlog)

func InstanceServerClientWithSlogWithInformativeErrFunc(isInformativeErrFunc func(error) bool) InstanceServerClientWithSlogOption {
	return func(_base *InstanceServerClientWithSlog) {
		_base._isInformativeErrFunc = isInformativeErrFunc
	}
}

// NewInstanceServerClientWithSlog instruments an implementation of the inventory.InstanceServerClient with simple logging.
func NewInstanceServerClientWithSlog(base inventory.InstanceServerClient, log *slog.Logger, opts ...InstanceServerClientWithSlogOption) InstanceServerClientWithSlog {
	this := InstanceServerClientWithSlog{
		_base:                 base,
		_log:                  log,
		_isInformativeErrFunc: func(error) bool { return false },
	}

	for _, opt := range opts {
		opt(&this)
	}

	return this
}

// GetInstanceByName implements inventory.InstanceServerClient.
func (_d InstanceServerClientWithSlog) GetInstanceByName(ctx context.Context, cluster provisioning.Cluster, instanceName string) (instanceFull api.InstanceFull, err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.Any("cluster", cluster),
			slog.String("instanceName", instanceName),
		)
	}
	log.Debug("=> calling GetInstanceByName")
	defer func() {
		log := _d._log.With()
		if _d._log.Enabled(ctx, logger.LevelTrace) {
			log = _d._log.With(
				slog.Any("instanceFull", instanceFull),
				slog.Any("err", err),
			)
		} else {
			if err != nil {
				log = _d._log.With("err", err)
			}
		}
		if err != nil {
			if _d._isInformativeErrFunc(err) {
				log.Debug("<= method GetInstanceByName returned an informative error")
			} else {
				log.Error("<= method GetInstanceByName returned an error")
			}
		} else {
			log.Debug("<= method GetInstanceByName finished")
		}
	}()
	return _d._base.GetInstanceByName(ctx, cluster, instanceName)
}

// GetInstances implements inventory.InstanceServerClient.
func (_d InstanceServerClientWithSlog) GetInstances(ctx context.Context, cluster provisioning.Cluster) (instanceFulls []api.InstanceFull, err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.Any("cluster", cluster),
		)
	}
	log.Debug("=> calling GetInstances")
	defer func() {
		log := _d._log.With()
		if _d._log.Enabled(ctx, logger.LevelTrace) {
			log = _d._log.With(
				slog.Any("instanceFulls", instanceFulls),
				slog.Any("err", err),
			)
		} else {
			if err != nil {
				log = _d._log.With("err", err)
			}
		}
		if err != nil {
			if _d._isInformativeErrFunc(err) {
				log.Debug("<= method GetInstances returned an informative error")
			} else {
				log.Error("<= method GetInstances returned an error")
			}
		} else {
			log.Debug("<= method GetInstances finished")
		}
	}()
	return _d._base.GetInstances(ctx, cluster)
}
