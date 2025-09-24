package provisioning

import (
	"context"
)

type ClusterTemplateService interface {
	Create(ctx context.Context, clusterTemplate ClusterTemplate) (ClusterTemplate, error)
	GetAll(ctx context.Context) (ClusterTemplates, error)
	GetAllNames(ctx context.Context) ([]string, error)
	GetByName(ctx context.Context, name string) (*ClusterTemplate, error)
	Update(ctx context.Context, clusterTemplate ClusterTemplate) error
	Rename(ctx context.Context, oldName string, newName string) error
	DeleteByName(ctx context.Context, name string) error
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
