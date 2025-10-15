package terraform

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lxc/incus/v6/shared/subprocess"
)

func (t terraform) terraformInit(ctx context.Context, configDir string) error {
	env := cleanEnvVars(os.Environ())

	// Make sure, terraform provider uses the client certificate of Operations Center.
	env = append(env, "INCUS_CONF="+t.clientCertDir)

	_, _, err := subprocess.RunCommandSplit(ctx, env, nil, "tofu", "-chdir="+configDir, "init", "-reconfigure")
	if err != nil {
		return fmt.Errorf(`Failed to run "tofu init -reconfigure": %w`, err)
	}

	return nil
}

func (t terraform) terraformApply(ctx context.Context, configDir string) error {
	env := cleanEnvVars(os.Environ())

	// Make sure, terraform provider uses the client certificate of Operations Center.
	env = append(env, "INCUS_CONF="+t.clientCertDir)

	_, _, err := subprocess.RunCommandSplit(ctx, env, nil, "tofu", "-chdir="+configDir, "apply", "-auto-approve")
	if err != nil {
		return fmt.Errorf(`Failed to run "tofu appy": %w`, err)
	}

	return nil
}

func cleanEnvVars(envVars []string) []string {
	cleanEnv := make([]string, 0, len(envVars))

	for _, envVar := range envVars {
		parts := strings.SplitN(envVar, "=", 2)
		name := parts[0]

		switch name {
		case "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy":
			// Skip the "well-known" proxy related env vars.
			continue
		}

		cleanEnv = append(cleanEnv, envVar)
	}

	return cleanEnv
}
