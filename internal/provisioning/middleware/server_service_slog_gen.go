// Code generated by mockery. DO NOT EDIT.
// template: github.com/FuturFusion/operations-center/internal/logger/slog.gotmpl

package middleware

import (
	"context"
	"log/slog"

	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/google/uuid"
)

// ServerServiceWithSlog implements provisioning.ServerService that is instrumented with slog logger.
type ServerServiceWithSlog struct {
	_log                  *slog.Logger
	_base                 provisioning.ServerService
	_isInformativeErrFunc func(error) bool
}

type ServerServiceWithSlogOption func(s *ServerServiceWithSlog)

func ServerServiceWithSlogWithInformativeErrFunc(isInformativeErrFunc func(error) bool) ServerServiceWithSlogOption {
	return func(_base *ServerServiceWithSlog) {
		_base._isInformativeErrFunc = isInformativeErrFunc
	}
}

// NewServerServiceWithSlog instruments an implementation of the provisioning.ServerService with simple logging.
func NewServerServiceWithSlog(base provisioning.ServerService, log *slog.Logger, opts ...ServerServiceWithSlogOption) ServerServiceWithSlog {
	this := ServerServiceWithSlog{
		_base:                 base,
		_log:                  log,
		_isInformativeErrFunc: func(error) bool { return false },
	}

	for _, opt := range opts {
		opt(&this)
	}

	return this
}

// Create implements provisioning.ServerService.
func (_d ServerServiceWithSlog) Create(ctx context.Context, token uuid.UUID, server provisioning.Server) (server1 provisioning.Server, err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.Any("token", token),
			slog.Any("server", server),
		)
	}
	log.Debug("=> calling Create")
	defer func() {
		log := _d._log.With()
		if _d._log.Enabled(ctx, logger.LevelTrace) {
			log = _d._log.With(
				slog.Any("server1", server1),
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
	return _d._base.Create(ctx, token, server)
}

// DeleteByName implements provisioning.ServerService.
func (_d ServerServiceWithSlog) DeleteByName(ctx context.Context, name string) (err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.String("name", name),
		)
	}
	log.Debug("=> calling DeleteByName")
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
				log.Debug("<= method DeleteByName returned an informative error")
			} else {
				log.Error("<= method DeleteByName returned an error")
			}
		} else {
			log.Debug("<= method DeleteByName finished")
		}
	}()
	return _d._base.DeleteByName(ctx, name)
}

// GetAll implements provisioning.ServerService.
func (_d ServerServiceWithSlog) GetAll(ctx context.Context) (servers provisioning.Servers, err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
		)
	}
	log.Debug("=> calling GetAll")
	defer func() {
		log := _d._log.With()
		if _d._log.Enabled(ctx, logger.LevelTrace) {
			log = _d._log.With(
				slog.Any("servers", servers),
				slog.Any("err", err),
			)
		} else {
			if err != nil {
				log = _d._log.With("err", err)
			}
		}
		if err != nil {
			if _d._isInformativeErrFunc(err) {
				log.Debug("<= method GetAll returned an informative error")
			} else {
				log.Error("<= method GetAll returned an error")
			}
		} else {
			log.Debug("<= method GetAll finished")
		}
	}()
	return _d._base.GetAll(ctx)
}

// GetAllNames implements provisioning.ServerService.
func (_d ServerServiceWithSlog) GetAllNames(ctx context.Context) (strings []string, err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
		)
	}
	log.Debug("=> calling GetAllNames")
	defer func() {
		log := _d._log.With()
		if _d._log.Enabled(ctx, logger.LevelTrace) {
			log = _d._log.With(
				slog.Any("strings", strings),
				slog.Any("err", err),
			)
		} else {
			if err != nil {
				log = _d._log.With("err", err)
			}
		}
		if err != nil {
			if _d._isInformativeErrFunc(err) {
				log.Debug("<= method GetAllNames returned an informative error")
			} else {
				log.Error("<= method GetAllNames returned an error")
			}
		} else {
			log.Debug("<= method GetAllNames finished")
		}
	}()
	return _d._base.GetAllNames(ctx)
}

// GetAllNamesWithFilter implements provisioning.ServerService.
func (_d ServerServiceWithSlog) GetAllNamesWithFilter(ctx context.Context, filter provisioning.ServerFilter) (strings []string, err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.Any("filter", filter),
		)
	}
	log.Debug("=> calling GetAllNamesWithFilter")
	defer func() {
		log := _d._log.With()
		if _d._log.Enabled(ctx, logger.LevelTrace) {
			log = _d._log.With(
				slog.Any("strings", strings),
				slog.Any("err", err),
			)
		} else {
			if err != nil {
				log = _d._log.With("err", err)
			}
		}
		if err != nil {
			if _d._isInformativeErrFunc(err) {
				log.Debug("<= method GetAllNamesWithFilter returned an informative error")
			} else {
				log.Error("<= method GetAllNamesWithFilter returned an error")
			}
		} else {
			log.Debug("<= method GetAllNamesWithFilter finished")
		}
	}()
	return _d._base.GetAllNamesWithFilter(ctx, filter)
}

// GetAllWithFilter implements provisioning.ServerService.
func (_d ServerServiceWithSlog) GetAllWithFilter(ctx context.Context, filter provisioning.ServerFilter) (servers provisioning.Servers, err error) {
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
				slog.Any("servers", servers),
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

// GetByName implements provisioning.ServerService.
func (_d ServerServiceWithSlog) GetByName(ctx context.Context, name string) (server *provisioning.Server, err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.String("name", name),
		)
	}
	log.Debug("=> calling GetByName")
	defer func() {
		log := _d._log.With()
		if _d._log.Enabled(ctx, logger.LevelTrace) {
			log = _d._log.With(
				slog.Any("server", server),
				slog.Any("err", err),
			)
		} else {
			if err != nil {
				log = _d._log.With("err", err)
			}
		}
		if err != nil {
			if _d._isInformativeErrFunc(err) {
				log.Debug("<= method GetByName returned an informative error")
			} else {
				log.Error("<= method GetByName returned an error")
			}
		} else {
			log.Debug("<= method GetByName finished")
		}
	}()
	return _d._base.GetByName(ctx, name)
}

// PollServers implements provisioning.ServerService.
func (_d ServerServiceWithSlog) PollServers(ctx context.Context, serverStatus api.ServerStatus, updateServerConfiguration bool) (err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.Any("serverStatus", serverStatus),
			slog.Bool("updateServerConfiguration", updateServerConfiguration),
		)
	}
	log.Debug("=> calling PollServers")
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
				log.Debug("<= method PollServers returned an informative error")
			} else {
				log.Error("<= method PollServers returned an error")
			}
		} else {
			log.Debug("<= method PollServers finished")
		}
	}()
	return _d._base.PollServers(ctx, serverStatus, updateServerConfiguration)
}

// Rename implements provisioning.ServerService.
func (_d ServerServiceWithSlog) Rename(ctx context.Context, oldName string, newName string) (err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.String("oldName", oldName),
			slog.String("newName", newName),
		)
	}
	log.Debug("=> calling Rename")
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
				log.Debug("<= method Rename returned an informative error")
			} else {
				log.Error("<= method Rename returned an error")
			}
		} else {
			log.Debug("<= method Rename finished")
		}
	}()
	return _d._base.Rename(ctx, oldName, newName)
}

// SelfUpdate implements provisioning.ServerService.
func (_d ServerServiceWithSlog) SelfUpdate(ctx context.Context, serverUpdate provisioning.ServerSelfUpdate) (err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.Any("serverUpdate", serverUpdate),
		)
	}
	log.Debug("=> calling SelfUpdate")
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
				log.Debug("<= method SelfUpdate returned an informative error")
			} else {
				log.Error("<= method SelfUpdate returned an error")
			}
		} else {
			log.Debug("<= method SelfUpdate finished")
		}
	}()
	return _d._base.SelfUpdate(ctx, serverUpdate)
}

// Update implements provisioning.ServerService.
func (_d ServerServiceWithSlog) Update(ctx context.Context, server provisioning.Server) (err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.Any("server", server),
		)
	}
	log.Debug("=> calling Update")
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
				log.Debug("<= method Update returned an informative error")
			} else {
				log.Error("<= method Update returned an error")
			}
		} else {
			log.Debug("<= method Update finished")
		}
	}()
	return _d._base.Update(ctx, server)
}

// UpdateSystemNetwork implements provisioning.ServerService.
func (_d ServerServiceWithSlog) UpdateSystemNetwork(ctx context.Context, name string, networkConfig provisioning.ServerSystemNetwork) (err error) {
	log := _d._log.With()
	if _d._log.Enabled(ctx, logger.LevelTrace) {
		log = log.With(
			slog.Any("ctx", ctx),
			slog.String("name", name),
			slog.Any("networkConfig", networkConfig),
		)
	}
	log.Debug("=> calling UpdateSystemNetwork")
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
				log.Debug("<= method UpdateSystemNetwork returned an informative error")
			} else {
				log.Error("<= method UpdateSystemNetwork returned an error")
			}
		} else {
			log.Debug("<= method UpdateSystemNetwork finished")
		}
	}()
	return _d._base.UpdateSystemNetwork(ctx, name, networkConfig)
}
