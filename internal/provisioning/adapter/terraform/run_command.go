package terraform

import (
	"context"
	"fmt"
	"os"

	"github.com/lxc/incus/v6/shared/subprocess"
)

func (t terraform) terraformInit(ctx context.Context, configDir string) error {
	env := os.Environ()
	// Make sure, terraform provider uses the client certificate of Operations Center.
	env = append(env, "INCUS_CONF="+t.clientCertDir)

	_, _, err := subprocess.RunCommandSplit(ctx, env, nil, "tofu", "-chdir="+configDir, "init", "-reconfigure")
	if err != nil {
		return fmt.Errorf(`Failed to run "tofu init -reconfigure": %w`, err)
	}

	return nil
}

func (t terraform) terraformApply(ctx context.Context, configDir string) error {
	env := os.Environ()
	// Make sure, terraform provider uses the client certificate of Operations Center.
	env = append(env, "INCUS_CONF="+t.clientCertDir)

	_, _, err := subprocess.RunCommandSplit(ctx, env, nil, "tofu", "-chdir="+configDir, "apply", "-auto-approve")
	if err != nil {
		return fmt.Errorf(`Failed to run "tofu appy": %w`, err)
	}

	return nil
}
