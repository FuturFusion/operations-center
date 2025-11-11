package provisioning

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
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

func (s clusterTemplateService) Apply(ctx context.Context, name string, templateVariables api.ConfigMap) (servicesConfig map[string]any, applicationSeedConfig map[string]any, _ error) {
	if name == "" {
		return nil, nil, fmt.Errorf("Cluster template name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	clusterTemplate, err := s.GetByName(ctx, name)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to get cluster template %q: %w", name, err)
	}

	servicesConfig = map[string]any{}
	applicationSeedConfig = map[string]any{}

	if templateVariables == nil {
		templateVariables = make(api.ConfigMap)
	}

	templates := []struct {
		name     string
		template string
		config   *map[string]any
	}{
		{
			name:     "service config",
			template: clusterTemplate.ServiceConfigTemplate,
			config:   &servicesConfig,
		},
		{
			name:     "application seed config",
			template: clusterTemplate.ApplicationConfigTemplate,
			config:   &applicationSeedConfig,
		},
	}

	for _, tmpl := range templates {
		serviceConfigFilled, err := applyVariables(tmpl.template, clusterTemplate.Variables, templateVariables)
		if err != nil {
			return nil, nil, domain.NewValidationErrf("Failed to apply cluster template variables on %s of %q: %v", tmpl.name, name, err)
		}

		err = yaml.Unmarshal([]byte(serviceConfigFilled), tmpl.config)
		if err != nil {
			return nil, nil, domain.NewValidationErrf("Failed to marshal %s of %q: %v", tmpl.name, name, err)
		}
	}

	return servicesConfig, applicationSeedConfig, nil
}

func applyVariables(template string, variables api.ClusterTemplateVariables, variableValues api.ConfigMap) (string, error) {
	for variableName, variableDefinition := range variables {
		_, ok := variableValues[variableName]
		if !ok {
			if variableDefinition.DefaultValue == "" {
				return "", fmt.Errorf("No value provided for variable %q, which is required, since it has no default value defined", variableName)
			}

			// Use default value.
			variableValues[variableName] = variableDefinition.DefaultValue
		}
	}

	for name, value := range variableValues {
		variableName := "@" + name + "@"
		template = strings.ReplaceAll(template, variableName, value)
	}

	return template, nil
}
