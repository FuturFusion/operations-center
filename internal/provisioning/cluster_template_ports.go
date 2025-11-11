package provisioning

import (
	"context"

	"github.com/FuturFusion/operations-center/shared/api"
)

type ClusterTemplateService interface {
	Create(ctx context.Context, clusterTemplate ClusterTemplate) (ClusterTemplate, error)
	GetAll(ctx context.Context) (ClusterTemplates, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*ClusterTemplate, error)
	Update(ctx context.Context, clusterTemplate ClusterTemplate) error
	Rename(ctx context.Context, oldName string, newName string) error
	DeleteByName(ctx context.Context, name string) error
	Apply(ctx context.Context, name string, templateVariables api.ConfigMap) (servicesConfig map[string]any, applicationSeedConfig map[string]any, _ error)
}

type ClusterTemplateRepo interface {
	Create(ctx context.Context, clusterTemplate ClusterTemplate) (int64, error)
	GetAll(ctx context.Context) (ClusterTemplates, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*ClusterTemplate, error)
	Update(ctx context.Context, clusterTemplate ClusterTemplate) error
	Rename(ctx context.Context, oldName string, newName string) error
	DeleteByName(ctx context.Context, name string) error
}
