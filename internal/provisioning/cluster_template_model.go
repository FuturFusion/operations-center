package provisioning

import (
	"regexp"
	"strings"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

type ClusterTemplate struct {
	ID                        int64
	Name                      string `db:"primary=yes"`
	Description               string
	ServiceConfigTemplate     string
	ApplicationConfigTemplate string
	Variables                 api.ClusterTemplateVariables
	LastUpdated               time.Time `db:"update_timestamp"`
}

var variablePattern = regexp.MustCompile("@[a-zA-Z0-9_]+@")

func (c ClusterTemplate) Validate() error {
	if c.Name == "" {
		return domain.NewValidationErrf("Invalid cluster template, name can not be empty")
	}

	if strings.ContainsAny(c.Name, nameProhibitedCharacters) {
		return domain.NewValidationErrf("Invalid cluster template, name can not contain any of %q", nameProhibitedCharacters)
	}

	for variable := range c.Variables {
		variableName := "@" + variable + "@"
		if !variablePattern.MatchString(variableName) {
			return domain.NewValidationErrf("Varible %q does not match the expected pattern or contains invalid characters", variable)
		}

		found := strings.Contains(c.ServiceConfigTemplate, variableName)
		if found {
			continue
		}

		found = strings.Contains(c.ApplicationConfigTemplate, variableName)
		if found {
			continue
		}

		return domain.NewValidationErrf("Defined variable %q is not used in any template", variable)
	}

	for _, variable := range variablePattern.FindAllString(c.ServiceConfigTemplate, -1) {
		variableName := variable[1 : len(variable)-1]
		_, ok := c.Variables[variableName]
		if !ok {
			return domain.NewValidationErrf("Variable %q used in the service config template is not contained in the variable definitions", variableName)
		}
	}

	for _, variable := range variablePattern.FindAllString(c.ApplicationConfigTemplate, -1) {
		variableName := variable[1 : len(variable)-1]
		_, ok := c.Variables[variableName]
		if !ok {
			return domain.NewValidationErrf("Variable %q used in the application config template is not contained in the variable definitions", variableName)
		}
	}

	return nil
}

type ClusterTemplates []ClusterTemplate
