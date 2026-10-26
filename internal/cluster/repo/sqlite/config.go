package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/cowsql/go-cowsql/cluster/db/transaction"

	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type configRepo struct {
	db transaction.Transactor
}

// CreateSharedConfig implements [cluster.ConfigRepo].
func (c *configRepo) CreateSharedConfig(ctx context.Context, cfg system.SharedConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	return transaction.ForceTx(ctx, c.db, func(ctx context.Context, tx transaction.TX) error {
		_, err := entities.CreateConfigSet(ctx, tx, entities.ConfigSet{Data: b})

		return err
	})
}

// GetSharedConfig implements [cluster.ConfigRepo].
func (c *configRepo) GetSharedConfig(ctx context.Context) (*system.SharedConfig, error) {
	var cfg *entities.ConfigSet
	err := transaction.ForceTx(ctx, c.db, func(ctx context.Context, tx transaction.TX) error {
		var err error
		cfg, err = entities.GetConfigSet(ctx, tx, sql.NullInt64{})
		if err != nil && !errors.Is(err, entities.ErrNotFound) {
			return err
		}

		// Initialize the config if none exists yet.
		if err != nil {
			err := c.CreateSharedConfig(ctx, config.ProvisionalShared())
			if err != nil {
				return err
			}

			cfg, err = entities.GetConfigSet(ctx, tx, sql.NullInt64{})
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return sharedConfigFromRepo(*cfg)
}

// UpdateSharedConfig implements [cluster.ConfigRepo].
func (c *configRepo) UpdateSharedConfig(ctx context.Context, cfg system.SharedConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	return transaction.ForceTx(ctx, c.db, func(ctx context.Context, tx transaction.TX) error {
		return entities.UpdateConfigSetData(ctx, tx, sql.NullInt64{}, b)
	})
}

func NewConfigRepo(db transaction.Transactor) *configRepo {
	return &configRepo{db: db}
}

func sharedConfigFromRepo(cfg entities.ConfigSet) (*system.SharedConfig, error) {
	if cfg.NodeID.Valid {
		return nil, errors.New("Config set is not shared")
	}

	var c system.SharedConfig
	err := json.Unmarshal(cfg.Data, &c)
	if err != nil {
		return nil, err
	}

	return &c, nil
}
