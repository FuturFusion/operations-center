package provisioning

import (
	"context"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/domain"
)

type clusterTemplateService struct {
	repo ClusterTemplateRepo
}

var _ ClusterTemplateService = &clusterTemplateService{}

func NewClusterTemplateService(
	repo ClusterTemplateRepo,
) *clusterTemplateService {
	clusterSvc := &clusterTemplateService{
		repo: repo,
	}

	return clusterSvc
}

func (s clusterTemplateService) Create(ctx context.Context, newClusterTemplate ClusterTemplate) (ClusterTemplate, error) {
	err := newClusterTemplate.Validate()
	if err != nil {
		return ClusterTemplate{}, err
	}

	newClusterTemplate.ID, err = s.repo.Create(ctx, newClusterTemplate)
	if err != nil {
		return ClusterTemplate{}, err
	}

	return newClusterTemplate, nil
}

func (s clusterTemplateService) GetAll(ctx context.Context) (ClusterTemplates, error) {
	return s.repo.GetAll(ctx)
}

func (s clusterTemplateService) GetAllNames(ctx context.Context) ([]string, error) {
	return s.repo.GetAllNames(ctx)
}

func (s clusterTemplateService) GetByName(ctx context.Context, name string) (*ClusterTemplate, error) {
	if name == "" {
		return nil, fmt.Errorf("Cluster template name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return s.repo.GetByName(ctx, name)
}

func (s clusterTemplateService) Update(ctx context.Context, newClusterTemplate ClusterTemplate) error {
	err := newClusterTemplate.Validate()
	if err != nil {
		return err
	}

	return s.repo.Update(ctx, newClusterTemplate)
}

func (s clusterTemplateService) Rename(ctx context.Context, oldName string, newName string) error {
	if oldName == "" {
		return fmt.Errorf("Cluster template name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	if newName == "" {
		return domain.NewValidationErrf("New cluster template name cannot by empty")
	}

	return s.repo.Rename(ctx, oldName, newName)
}

func (s clusterTemplateService) DeleteByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("Cluster template name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	err := s.repo.DeleteByName(ctx, name)
	if err != nil {
		return fmt.Errorf("Failed to delete cluster template: %w", err)
	}

	return nil
}
