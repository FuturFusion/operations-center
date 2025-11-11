package provisioning_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestClusterTemplateService_Create(t *testing.T) {
	tests := []struct {
		name            string
		clusterTemplate provisioning.ClusterTemplate
		repoCreateErr   error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			clusterTemplate: provisioning.ClusterTemplate{
				Name: "A",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - validation",
			clusterTemplate: provisioning.ClusterTemplate{
				Name: "", // invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - repo.Create",
			clusterTemplate: provisioning.ClusterTemplate{
				Name: "A",
			},
			repoCreateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterTemplateRepoMock{
				CreateFunc: func(ctx context.Context, in provisioning.ClusterTemplate) (int64, error) {
					return 1, tc.repoCreateErr
				},
			}

			clusterTemplateSvc := provisioning.NewClusterTemplateService(repo)

			// Run test
			_, err := clusterTemplateSvc.Create(t.Context(), tc.clusterTemplate)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterTemplateService_GetAll(t *testing.T) {
	tests := []struct {
		name                       string
		repoGetAllClusterTemplates provisioning.ClusterTemplates
		repoGetAllErr              error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllClusterTemplates: provisioning.ClusterTemplates{
				provisioning.ClusterTemplate{
					Name: "A",
				},
				provisioning.ClusterTemplate{
					Name: "B",
				},
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:          "error - repo",
			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterTemplateRepoMock{
				GetAllFunc: func(ctx context.Context) (provisioning.ClusterTemplates, error) {
					return tc.repoGetAllClusterTemplates, tc.repoGetAllErr
				},
			}

			clusterTemplateSvc := provisioning.NewClusterTemplateService(repo)

			// Run test
			clusterTemplates, err := clusterTemplateSvc.GetAll(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, clusterTemplates, tc.count)
		})
	}
}

func TestClusterTemplateService_GetAllNames(t *testing.T) {
	tests := []struct {
		name               string
		repoGetAllNames    []string
		repoGetAllNamesErr error

		assertErr require.ErrorAssertionFunc
		count     int
	}{
		{
			name: "success",
			repoGetAllNames: []string{
				"A",
				"B",
			},

			assertErr: require.NoError,
			count:     2,
		},
		{
			name:               "error - repo",
			repoGetAllNamesErr: boom.Error,

			assertErr: boom.ErrorIs,
			count:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterTemplateRepoMock{
				GetAllNamesFunc: func(ctx context.Context) ([]string, error) {
					return tc.repoGetAllNames, tc.repoGetAllNamesErr
				},
			}

			clusterTemplateSvc := provisioning.NewClusterTemplateService(repo)

			// Run test
			clusterTemplateIDs, err := clusterTemplateSvc.GetAllNames(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Len(t, clusterTemplateIDs, tc.count)
		})
	}
}

func TestClusterTemplateService_GetByName(t *testing.T) {
	tests := []struct {
		name                         string
		nameArg                      string
		repoGetByNameClusterTemplate *provisioning.ClusterTemplate
		repoGetByNameErr             error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "A",
			repoGetByNameClusterTemplate: &provisioning.ClusterTemplate{
				Name: "A",
			},

			assertErr: require.NoError,
		},
		{
			name:    "error - empty name",
			nameArg: "",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:             "error - repo",
			nameArg:          "A",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterTemplateRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.ClusterTemplate, error) {
					return tc.repoGetByNameClusterTemplate, tc.repoGetByNameErr
				},
			}

			clusterTempalteSvc := provisioning.NewClusterTemplateService(repo)

			// Run test
			clusterTempalte, err := clusterTempalteSvc.GetByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.repoGetByNameClusterTemplate, clusterTempalte)
		})
	}
}

func TestClusterTemplateService_Update(t *testing.T) {
	tests := []struct {
		name            string
		clusterTemplate provisioning.ClusterTemplate
		repoUpdateErr   error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			clusterTemplate: provisioning.ClusterTemplate{
				Name: "A",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - invalid name",
			clusterTemplate: provisioning.ClusterTemplate{
				Name: "", // invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - repo",
			clusterTemplate: provisioning.ClusterTemplate{
				Name: "A",
			},
			repoUpdateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterTemplateRepoMock{
				UpdateFunc: func(ctx context.Context, in provisioning.ClusterTemplate) error {
					return tc.repoUpdateErr
				},
			}

			clusterTempalteSvc := provisioning.NewClusterTemplateService(repo)

			// Run test
			err := clusterTempalteSvc.Update(t.Context(), tc.clusterTemplate)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterTemplateService_Rename(t *testing.T) {
	tests := []struct {
		name          string
		oldName       string
		newName       string
		repoRenameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			oldName: "one",
			newName: "one new",

			assertErr: require.NoError,
		},
		{
			name:    "error - old name empty",
			oldName: "", // invalid
			newName: "one new",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:    "error - new name empty",
			oldName: "one",
			newName: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name:          "error - repo.Rename",
			oldName:       "one",
			newName:       "one new",
			repoRenameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterTemplateRepoMock{
				RenameFunc: func(ctx context.Context, oldName string, newName string) error {
					require.Equal(t, tc.oldName, oldName)
					require.Equal(t, tc.newName, newName)
					return tc.repoRenameErr
				},
			}

			clusterTemplateSvc := provisioning.NewClusterTemplateService(repo)

			// Run test
			err := clusterTemplateSvc.Rename(t.Context(), tc.oldName, tc.newName)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterTemplateService_DeleteByName(t *testing.T) {
	tests := []struct {
		name                string
		nameArg             string
		repoDeleteByNameErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "A",

			assertErr: require.NoError,
		},
		{
			name:    "error - empty name",
			nameArg: "",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted, a...)
			},
		},
		{
			name:                "error - repo",
			nameArg:             "A",
			repoDeleteByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterTemplateRepoMock{
				DeleteByNameFunc: func(ctx context.Context, id string) error {
					return tc.repoDeleteByNameErr
				},
			}

			clusterTemplateSvc := provisioning.NewClusterTemplateService(repo)

			// Run test
			err := clusterTemplateSvc.DeleteByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestClusterTemplateService_Apply(t *testing.T) {
	tests := []struct {
		name              string
		nameArg           string
		templateVariables api.ConfigMap
		repoGetByName     *provisioning.ClusterTemplate
		repoGetByNameErr  error

		assertErr                 require.ErrorAssertionFunc
		wantServicesConfig        map[string]any
		wantApplicationSeedConfig map[string]any
	}{
		{
			name:    "success - empty template",
			nameArg: "tmpl",
			repoGetByName: &provisioning.ClusterTemplate{
				ServiceConfigTemplate:     ``,
				ApplicationConfigTemplate: ``,
				Variables:                 api.ClusterTemplateVariables{},
			},

			assertErr:                 require.NoError,
			wantServicesConfig:        map[string]any{},
			wantApplicationSeedConfig: map[string]any{},
		},
		{
			name:    "success - with template",
			nameArg: "tmpl",
			templateVariables: api.ConfigMap{
				"VALUE": "template value",
			},
			repoGetByName: &provisioning.ClusterTemplate{
				ServiceConfigTemplate: `
key: @VALUE@
static: static value
default: @VALUE_WITH_DEFAULT@
`,
				ApplicationConfigTemplate: ``,
				Variables: api.ClusterTemplateVariables{
					"VALUE": api.ClusterTemplateVariable{},
					"VALUE_WITH_DEFAULT": api.ClusterTemplateVariable{
						DefaultValue: "default value",
					},
				},
			},

			assertErr: require.NoError,
			wantServicesConfig: map[string]any{
				"key":     "template value",
				"static":  "static value",
				"default": "default value",
			},
			wantApplicationSeedConfig: map[string]any{},
		},
		{
			name:    "error - name missing",
			nameArg: "", // invalid

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
				require.ErrorContains(tt, err, `Cluster template name cannot be empty`)
			},
		},
		{
			name:             "error - name missing",
			nameArg:          "tmpl",
			repoGetByNameErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:              "error - service config apply variables",
			nameArg:           "tmpl",
			templateVariables: api.ConfigMap{},
			repoGetByName: &provisioning.ClusterTemplate{
				ServiceConfigTemplate: `
key: @VALUE@
`,
				ApplicationConfigTemplate: ``,
				Variables: api.ClusterTemplateVariables{
					"VALUE": api.ClusterTemplateVariable{},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Failed to apply cluster template variables on service config of")
			},
		},
		{
			name:    "error - service config  yaml unmarshal",
			nameArg: "tmpl",
			repoGetByName: &provisioning.ClusterTemplate{
				ServiceConfigTemplate: `
[
`, // invalid yaml
				ApplicationConfigTemplate: ``,
				Variables:                 api.ClusterTemplateVariables{},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Failed to marshal service config of")
			},
		},
		{
			name:    "error - application seed config yaml unmarshal",
			nameArg: "tmpl",
			repoGetByName: &provisioning.ClusterTemplate{
				ServiceConfigTemplate: ``,
				ApplicationConfigTemplate: `
		[
		`, // invalid yaml
				Variables: api.ClusterTemplateVariables{},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Failed to marshal application seed config of")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.ClusterTemplateRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.ClusterTemplate, error) {
					require.Equal(t, tc.nameArg, name)
					return tc.repoGetByName, tc.repoGetByNameErr
				},
			}

			clusterTemplateSvc := provisioning.NewClusterTemplateService(repo)

			// Run test
			servicesConfig, applicationSeedConfig, err := clusterTemplateSvc.Apply(t.Context(), tc.nameArg, tc.templateVariables)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantServicesConfig, servicesConfig)
			require.Equal(t, tc.wantApplicationSeedConfig, applicationSeedConfig)
		})
	}
}
