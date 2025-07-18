// Code generated by mockery. DO NOT EDIT.
// template: github.com/FuturFusion/operations-center/internal/logger/slog.gotmpl

package middleware

import (
	"context"
	"log/slog"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/google/uuid"
)

// NetworkZoneRepoWithSlog implements inventory.NetworkZoneRepo that is instrumented with slog logger.
type NetworkZoneRepoWithSlog struct {
	_log                  *slog.Logger
	_base                 inventory.NetworkZoneRepo
	_isInformativeErrFunc func(error) bool
}

type NetworkZoneRepoWithSlogOption func(s *NetworkZoneRepoWithSlog)

func NetworkZoneRepoWithSlogWithInformativeErrFunc(isInformativeErrFunc func(error) bool) NetworkZoneRepoWithSlogOption {
	return func(_base *NetworkZoneRepoWithSlog) {
		_base._isInformativeErrFunc = isInformativeErrFunc
	}
}

// NewNetworkZoneRepoWithSlog instruments an implementation of the inventory.NetworkZoneRepo with simple logging.
func NewNetworkZoneRepoWithSlog(base inventory.NetworkZoneRepo, log *slog.Logger, opts ...NetworkZoneRepoWithSlogOption) NetworkZoneRepoWithSlog {
	this := NetworkZoneRepoWithSlog{
		_base:                 base,
		_log:                  log,
		_isInformativeErrFunc: func(error) bool { return false },
	}

	for _, opt := range opts {
		opt(&this)
	}

	return this
}

// Create implements inventory.NetworkZoneRepo.
func (_d NetworkZoneRepoWithSlog) Create(ctx context.Context, networkZone inventory.NetworkZone) (networkZone1 inventory.NetworkZone, err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.Any("networkZone", networkZone),
		)
	}
	log.Debug("=> calling Create")
	defer func() {
		log := _d._log.With()
		if _d._log.Enabled(ctx, logger.LevelTrace) {
			log = _d._log.With(
				slog.Any("networkZone1", networkZone1),
				slog.Any("err", err),
			)
		} else {
			if err != nil {
				log = _d._log.With("err", err)
			}
		}
		if err != nil {
			if _d._isInformativeErrFunc(err) {
				log.Debug("<= method Create returned an informative error")
			} else {
				log.Error("<= method Create returned an error")
			}
		} else {
			log.Debug("<= method Create finished")
		}
	}()
	return _d._base.Create(ctx, networkZone)
}

// DeleteByClusterName implements inventory.NetworkZoneRepo.
func (_d NetworkZoneRepoWithSlog) DeleteByClusterName(ctx context.Context, cluster string) (err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.String("cluster", cluster),
		)
	}
	log.Debug("=> calling DeleteByClusterName")
	defer func() {
		log := _d._log.With()
		if _d._log.Enabled(ctx, logger.LevelTrace) {
			log = _d._log.With(
				slog.Any("err", err),
			)
		} else {
			if err != nil {
				log = _d._log.With("err", err)
			}
		}
		if err != nil {
			if _d._isInformativeErrFunc(err) {
				log.Debug("<= method DeleteByClusterName returned an informative error")
			} else {
				log.Error("<= method DeleteByClusterName returned an error")
			}
		} else {
			log.Debug("<= method DeleteByClusterName finished")
		}
	}()
	return _d._base.DeleteByClusterName(ctx, cluster)
}

// DeleteByUUID implements inventory.NetworkZoneRepo.
func (_d NetworkZoneRepoWithSlog) DeleteByUUID(ctx context.Context, id uuid.UUID) (err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.Any("id", id),
		)
	}
	log.Debug("=> calling DeleteByUUID")
	defer func() {
		log := _d._log.With()
		if _d._log.Enabled(ctx, logger.LevelTrace) {
			log = _d._log.With(
				slog.Any("err", err),
			)
		} else {
			if err != nil {
				log = _d._log.With("err", err)
			}
		}
		if err != nil {
			if _d._isInformativeErrFunc(err) {
				log.Debug("<= method DeleteByUUID returned an informative error")
			} else {
				log.Error("<= method DeleteByUUID returned an error")
			}
		} else {
			log.Debug("<= method DeleteByUUID finished")
		}
	}()
	return _d._base.DeleteByUUID(ctx, id)
}

// GetAllUUIDsWithFilter implements inventory.NetworkZoneRepo.
func (_d NetworkZoneRepoWithSlog) GetAllUUIDsWithFilter(ctx context.Context, filter inventory.NetworkZoneFilter) (uUIDs []uuid.UUID, err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.Any("filter", filter),
		)
	}
	log.Debug("=> calling GetAllUUIDsWithFilter")
	defer func() {
		log := _d._log.With()
		if _d._log.Enabled(ctx, logger.LevelTrace) {
			log = _d._log.With(
				slog.Any("uUIDs", uUIDs),
				slog.Any("err", err),
			)
		} else {
			if err != nil {
				log = _d._log.With("err", err)
			}
		}
		if err != nil {
			if _d._isInformativeErrFunc(err) {
				log.Debug("<= method GetAllUUIDsWithFilter returned an informative error")
			} else {
				log.Error("<= method GetAllUUIDsWithFilter returned an error")
			}
		} else {
			log.Debug("<= method GetAllUUIDsWithFilter finished")
		}
	}()
	return _d._base.GetAllUUIDsWithFilter(ctx, filter)
}

// GetAllWithFilter implements inventory.NetworkZoneRepo.
func (_d NetworkZoneRepoWithSlog) GetAllWithFilter(ctx context.Context, filter inventory.NetworkZoneFilter) (networkZones inventory.NetworkZones, err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.Any("filter", filter),
		)
	}
	log.Debug("=> calling GetAllWithFilter")
	defer func() {
		log := _d._log.With()
		if _d._log.Enabled(ctx, logger.LevelTrace) {
			log = _d._log.With(
				slog.Any("networkZones", networkZones),
				slog.Any("err", err),
			)
		} else {
			if err != nil {
				log = _d._log.With("err", err)
			}
		}
		if err != nil {
			if _d._isInformativeErrFunc(err) {
				log.Debug("<= method GetAllWithFilter returned an informative error")
			} else {
				log.Error("<= method GetAllWithFilter returned an error")
			}
		} else {
			log.Debug("<= method GetAllWithFilter finished")
		}
	}()
	return _d._base.GetAllWithFilter(ctx, filter)
}

// GetByUUID implements inventory.NetworkZoneRepo.
func (_d NetworkZoneRepoWithSlog) GetByUUID(ctx context.Context, id uuid.UUID) (networkZone inventory.NetworkZone, err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.Any("id", id),
		)
	}
	log.Debug("=> calling GetByUUID")
	defer func() {
		log := _d._log.With()
		if _d._log.Enabled(ctx, logger.LevelTrace) {
			log = _d._log.With(
				slog.Any("networkZone", networkZone),
				slog.Any("err", err),
			)
		} else {
			if err != nil {
				log = _d._log.With("err", err)
			}
		}
		if err != nil {
			if _d._isInformativeErrFunc(err) {
				log.Debug("<= method GetByUUID returned an informative error")
			} else {
				log.Error("<= method GetByUUID returned an error")
			}
		} else {
			log.Debug("<= method GetByUUID finished")
		}
	}()
	return _d._base.GetByUUID(ctx, id)
}

// UpdateByUUID implements inventory.NetworkZoneRepo.
func (_d NetworkZoneRepoWithSlog) UpdateByUUID(ctx context.Context, networkZone inventory.NetworkZone) (networkZone1 inventory.NetworkZone, err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.Any("networkZone", networkZone),
		)
	}
	log.Debug("=> calling UpdateByUUID")
	defer func() {
		log := _d._log.With()
		if _d._log.Enabled(ctx, logger.LevelTrace) {
			log = _d._log.With(
				slog.Any("networkZone1", networkZone1),
				slog.Any("err", err),
			)
		} else {
			if err != nil {
				log = _d._log.With("err", err)
			}
		}
		if err != nil {
			if _d._isInformativeErrFunc(err) {
				log.Debug("<= method UpdateByUUID returned an informative error")
			} else {
				log.Error("<= method UpdateByUUID returned an error")
			}
		} else {
			log.Debug("<= method UpdateByUUID finished")
		}
	}()
	return _d._base.UpdateByUUID(ctx, networkZone)
}
