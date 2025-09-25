package provisioning_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestClusterTemplate_Validate(t *testing.T) {
	tests := []struct {
		name            string
		clusterTemplate provisioning.ClusterTemplate

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			clusterTemplate: provisioning.ClusterTemplate{
				Name: "one",
				ServiceConfigTemplate: `{
  "string": "@SERVICE_STRING@",
  "enabled": @SERVICE_ENABLED@,
  "number": @SERVICE_NUMBER@
}`,
				ApplicationConfigTemplate: `{
  "string": "@APPLICATION_STRING@",
  "enabled": @APPLICATION_ENABLED@,
  "number": @APPLICATION_NUMBER@
}`,
				Variables: api.ClusterTemplateVariables{
					"SERVICE_STRING": api.ClusterTemplateVariable{
						Description:  "service string",
						DefaultValue: "foobar",
					},
					"SERVICE_ENABLED": api.ClusterTemplateVariable{
						Description:  "service enabled",
						DefaultValue: "true",
					},
					"SERVICE_NUMBER": api.ClusterTemplateVariable{
						Description:  "service number",
						DefaultValue: "5",
					},
					"APPLICATION_STRING": api.ClusterTemplateVariable{
						Description:  "application string",
						DefaultValue: "foobar",
					},
					"APPLICATION_ENABLED": api.ClusterTemplateVariable{
						Description:  "application enabled",
						DefaultValue: "true",
					},
					"APPLICATION_NUMBER": api.ClusterTemplateVariable{
						Description:  "application number",
						DefaultValue: "5",
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name: "error - name empty",
			clusterTemplate: provisioning.ClusterTemplate{
				Name: "", // invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - name prohibited character",
			clusterTemplate: provisioning.ClusterTemplate{
				Name: "foo/bar", // "/" is prohibited
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - variable with invalid name",
			clusterTemplate: provisioning.ClusterTemplate{
				Name:                      "one",
				ServiceConfigTemplate:     `{}`,
				ApplicationConfigTemplate: `{}`,
				Variables: api.ClusterTemplateVariables{
					"INVALID+VARIABLE+NAME": api.ClusterTemplateVariable{}, // Variable with invalid name.
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - variable not used",
			clusterTemplate: provisioning.ClusterTemplate{
				Name:                      "one",
				ServiceConfigTemplate:     `{}`,
				ApplicationConfigTemplate: `{}`,
				Variables: api.ClusterTemplateVariables{
					"NOT_USED_VARIABLE": api.ClusterTemplateVariable{}, // Variable not used.
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - variable in service config template not defined",
			clusterTemplate: provisioning.ClusterTemplate{
				Name: "one",
				ServiceConfigTemplate: `{
  "string": "@NOT_DEFINED@",
}`,
				ApplicationConfigTemplate: `{}`,
				Variables:                 api.ClusterTemplateVariables{}, // variable @NOT_DEFINED@ is not present
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - variable in application config template not defined",
			clusterTemplate: provisioning.ClusterTemplate{
				Name:                  "one",
				ServiceConfigTemplate: `{}`,
				ApplicationConfigTemplate: `{
  "string": "@NOT_DEFINED@",
}`,
				Variables: api.ClusterTemplateVariables{}, // variable @NOT_DEFINED@ is not present
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.clusterTemplate.Validate()

			tc.assertErr(t, err)
		})
	}
}
