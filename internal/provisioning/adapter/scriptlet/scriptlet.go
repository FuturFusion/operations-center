package scriptlet

import (
	"context"
	"fmt"

	"github.com/lxc/incus/v6/shared/scriptlet"
	_ "go.starlark.net/starlark"

	daemonConfig "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type ScriptletClientPort interface {
	GetSystem(ctx context.Context, server provisioning.Server, resource string) (map[string]any, error)
	UpdateSystem(ctx context.Context, server provisioning.Server, resource string, config any) error
}

type Runner struct {
	loader *scriptlet.Loader

	client ScriptletClientPort
}

var _ provisioning.ServerScriptletPort = Runner{}

func New(loader *scriptlet.Loader, client ScriptletClientPort) (Runner, error) {
	err := loader.Set(serverRegistrationCompile, serverRegistration, daemonConfig.GetSettings().ServerRegistrationScriptlet)
	if err != nil {
		return Runner{}, fmt.Errorf("Failed to load server registration scriptlet: %w", err)
	}

	lifecycle.SettingsValidateSignal.AddListenerWithErr(func(ctx context.Context, settings system.Settings) error {
		err := serverRegistrationValidate(settings.ServerRegistrationScriptlet)
		if err != nil {
			return fmt.Errorf("Failed to validate server registration scriptlet: %w", err)
		}

		return nil
	})

	lifecycle.SettingsUpdateSignal.AddListenerWithErr(func(ctx context.Context, settings system.Settings) error {
		err := loader.Set(serverRegistrationCompile, serverRegistration, settings.ServerRegistrationScriptlet)
		if err != nil {
			return fmt.Errorf("Failed to compile and cache server registration scriptlet: %w", err)
		}

		return nil
	})

	return Runner{
		loader: loader,
		client: client,
	}, nil
}
