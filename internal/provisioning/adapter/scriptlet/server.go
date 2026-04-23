package scriptlet

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"

	"github.com/lxc/incus/v6/shared/scriptlet"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

const serverRegistration = "server_registration"

// serverRegistrationValidate validates the server registration scriptlet.
func serverRegistrationValidate(src string) error {
	if src == "" {
		return nil
	}

	err := scriptlet.Validate(serverRegistrationCompile, serverRegistration, src, scriptlet.Declaration{
		scriptlet.Required(serverRegistration): {"candidate"},
	})
	if err != nil {
		return fmt.Errorf("Failed to validate server registration scriptlet: %w", err)
	}

	return nil
}

// serverRegistrationCompile compiles the server registration scriptlet.
func serverRegistrationCompile(name string, src string) (*starlark.Program, error) {
	return scriptlet.Compile(name, src, []string{
		"log",
		"server",
		"incusos",
	})
}

// ServerRegistrationRun executes the server registration scriptlet.
func (r Runner) ServerRegistrationRun(ctx context.Context, server *provisioning.Server) error {
	if server == nil {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	prog, thread, err := r.loader.Program("Server registration", serverRegistration)
	if err != nil {
		// The loader does not provide an error type, that can be asserted other than string matching.
		if strings.Contains(err.Error(), "scriptlet not loaded") {
			return nil
		}

		return err
	}

	logFunc := CreateLogger(slog.Default(), "Server registration scriptlet")

	logNamespace := starlarkstruct.FromStringDict(
		starlarkstruct.Default,
		starlark.StringDict{
			"info":  starlark.NewBuiltin("info", logFunc),
			"warn":  starlark.NewBuiltin("warn", logFunc),
			"error": starlark.NewBuiltin("error", logFunc),
		},
	)

	serverNamespace := starlarkstruct.FromStringDict(
		starlarkstruct.Default,
		starlark.StringDict{
			"set_name":           starlark.NewBuiltin("set_name", setServerName(ctx, server)),
			"set_description":    starlark.NewBuiltin("set_description", setServerDescription(ctx, server)),
			"set_properties":     starlark.NewBuiltin("set_properties", setServerProperties(ctx, server)),
			"set_connection_url": starlark.NewBuiltin("set_connection_url", setServerConnectionURL(ctx, server)),
			"set_update_channel": starlark.NewBuiltin("set_update_channel", setServerUpdateChannel(ctx, server)),
		},
	)

	incusosNamespace := starlarkstruct.FromStringDict(
		starlarkstruct.Default,
		starlark.StringDict{
			"get_system": starlark.NewBuiltin("get_system", r.getIncusOSSystem(ctx, *server)),
			"set_system": starlark.NewBuiltin("set_system", r.setIncusOSSystem(ctx, *server)),

			"trigger_action": starlark.NewBuiltin("trigger_action", r.triggerIncusOSAction(ctx, *server)),

			"get_service": starlark.NewBuiltin("get_service", r.getIncusOSService(ctx, *server)),
			"set_service": starlark.NewBuiltin("set_service", r.setIncusOSService(ctx, *server)),

			"add_application": starlark.NewBuiltin("add_application", r.addIncusOSApplication(ctx, *server)),
		},
	)

	env := starlark.StringDict{
		"log":     logNamespace,
		"server":  serverNamespace,
		"incusos": incusosNamespace,
	}

	go func() {
		<-ctx.Done()
		thread.Cancel("Request finished")
	}()

	globals, err := prog.Init(thread, env)
	if err != nil {
		return fmt.Errorf("Failed initializing: %w", err)
	}

	globals.Freeze()

	// Retrieve a global variable from starlark environment.
	placement := globals[serverRegistration]
	if placement == nil {
		return fmt.Errorf("Scriptlet missing %q function", serverRegistration)
	}

	serverv, err := scriptlet.StarlarkMarshal(server)
	if err != nil {
		return fmt.Errorf("Marshalling request failed: %w", err)
	}

	v, err := starlark.Call(thread, placement, nil, []starlark.Tuple{
		{starlark.String("candidate"), serverv},
	})
	if err != nil {
		return fmt.Errorf("Failed to run: %w", err)
	}

	if v.Type() != "NoneType" {
		return fmt.Errorf("Failed with unexpected return value: %v", v)
	}

	return err
}

func setServerName(ctx context.Context, server *provisioning.Server) func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var serverName string
		err := starlark.UnpackArgs(b.Name(), args, kwargs, "server_name", &serverName)
		if err != nil {
			return nil, err
		}

		if serverName == "" {
			slog.ErrorContext(ctx, "Server registration scriptlet failed. Server name is empty")
			return nil, errors.New("Server name is empty")
		}

		server.Name = serverName
		slog.InfoContext(ctx, "Server registration scriptlet assigned name for server", slog.String("name", server.Name))

		return starlark.None, nil
	}
}

func setServerDescription(ctx context.Context, server *provisioning.Server) func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var serverDescription string
		err := starlark.UnpackArgs(b.Name(), args, kwargs, "server_description", &serverDescription)
		if err != nil {
			return nil, err
		}

		server.Description = serverDescription
		slog.InfoContext(ctx, "Server registration scriptlet assigned description for server", slog.String("name", server.Name), slog.String("description", server.Description))

		return starlark.None, nil
	}
}

func setServerConnectionURL(ctx context.Context, server *provisioning.Server) func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var serverConnectionURLStr string
		var public bool
		err := starlark.UnpackArgs(b.Name(), args, kwargs, "server_connection_url", &serverConnectionURLStr, "public", &public)
		if err != nil {
			return nil, err
		}

		if serverConnectionURLStr == "" {
			slog.ErrorContext(ctx, "Server registration scriptlet failed. Server connection URL is empty")
			return nil, errors.New("Connection URL is empty")
		}

		serverConnectionURL, err := url.Parse(serverConnectionURLStr)
		if err != nil {
			slog.ErrorContext(ctx, "Server registration scriptlet failed. Server connection URL is not valid")
			return nil, fmt.Errorf("Invalid connection URL: %w", err)
		}

		if serverConnectionURL.Scheme != "https" {
			slog.ErrorContext(ctx, "Server registration scriptlet failed. Server connection URL, schema is not https")
			return nil, errors.New("Invalid connection URL, schema is not https")
		}

		serverConnectionURLStr = "https://" + serverConnectionURL.Host
		if serverConnectionURL.Port() == "" {
			serverConnectionURLStr = "https://" + net.JoinHostPort(serverConnectionURL.Hostname(), "8443")
		}

		if public {
			server.PublicConnectionURL = serverConnectionURLStr
		} else {
			server.ConnectionURL = serverConnectionURLStr
		}

		slog.InfoContext(ctx, "Server registration scriptlet assigned connection URL for server", slog.String("name", server.Name), slog.String("connection_url", server.ConnectionURL), slog.Bool("public", public))

		return starlark.None, nil
	}
}

func setServerProperties(ctx context.Context, server *provisioning.Server) func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		props := &starlark.Dict{}
		err := starlark.UnpackArgs(b.Name(), args, kwargs, "properties", &props)
		if err != nil {
			return nil, err
		}

		properties := make(api.ConfigMap, props.Len())

		for _, slKey := range props.Keys() {
			key, ok := starlark.AsString(slKey)
			if !ok {
				return starlark.None, fmt.Errorf(`%q is an unexpected property key type, require "string"`, slKey.Type())
			}

			slValue, _, _ := props.Get(slKey)
			value, ok := starlark.AsString(slValue)
			if !ok {
				return starlark.None, fmt.Errorf(`%q is an unexpected property value type, require "string"`, slValue.Type())
			}

			properties[key] = value
		}

		server.Properties = properties
		slog.InfoContext(ctx, "Server registration scriptlet assigned properties for server", slog.String("name", server.Name), slog.Any("properties", server.Properties))

		return starlark.None, nil
	}
}

func setServerUpdateChannel(ctx context.Context, server *provisioning.Server) func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var serverUpdateChannel string
		err := starlark.UnpackArgs(b.Name(), args, kwargs, "server_update_channel", &serverUpdateChannel)
		if err != nil {
			return nil, err
		}

		server.Channel = serverUpdateChannel
		slog.InfoContext(ctx, "Server registration scriptlet assigned update channel for server", slog.String("name", server.Name), slog.Any("channel", server.Channel))

		return starlark.None, nil
	}
}

func (r Runner) getIncusOSSystem(ctx context.Context, server provisioning.Server) func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var resource string
		err := starlark.UnpackArgs(b.Name(), args, kwargs, "resource", &resource)
		if err != nil {
			return nil, err
		}

		res, err := r.client.GetSystem(ctx, server, resource)
		if err != nil {
			return nil, fmt.Errorf("Failed to get system %s information from server %q: %w", resource, server.Name, err)
		}

		rv, err := scriptlet.StarlarkMarshal(res)
		if err != nil {
			return nil, fmt.Errorf("Failed to marshal value for starlark: %w", err)
		}

		return rv, nil
	}
}

func (r Runner) setIncusOSSystem(ctx context.Context, server provisioning.Server) func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var resource string
		configArg := &starlark.Dict{}
		err := starlark.UnpackArgs(b.Name(), args, kwargs, "resource", &resource, "config", &configArg)
		if err != nil {
			return nil, err
		}

		config, err := scriptlet.StarlarkUnmarshal(configArg)
		if err != nil {
			return nil, err
		}

		err = r.client.UpdateSystem(ctx, server, resource, config)
		if err != nil {
			return starlark.None, fmt.Errorf("Failed to set system %s configuration for server %q: %w", resource, server.Name, err)
		}

		return starlark.None, nil
	}
}

func (r Runner) triggerIncusOSAction(ctx context.Context, server provisioning.Server) func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var resource string
		var action string
		bodyArg := &starlark.Dict{}
		err := starlark.UnpackArgs(b.Name(), args, kwargs, "resource", &resource, "action", &action, "body", &bodyArg)
		if err != nil {
			return nil, err
		}

		body, err := scriptlet.StarlarkUnmarshal(bodyArg)
		if err != nil {
			return nil, err
		}

		err = r.client.TriggerSystemAction(ctx, server, resource, action, body)
		if err != nil {
			return starlark.None, fmt.Errorf("Failed to execute system command %s/%s for server %q: %w", resource, action, server.Name, err)
		}

		return starlark.None, nil
	}
}

func (r Runner) getIncusOSService(ctx context.Context, server provisioning.Server) func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var service string
		err := starlark.UnpackArgs(b.Name(), args, kwargs, "service", &service)
		if err != nil {
			return nil, err
		}

		res, err := r.client.GetOSService(ctx, server, service)
		if err != nil {
			return nil, fmt.Errorf("Failed to get service %s configuration from server %q: %w", service, server.Name, err)
		}

		rv, err := scriptlet.StarlarkMarshal(res)
		if err != nil {
			return nil, fmt.Errorf("Failed to marshal value for starlark: %w", err)
		}

		return rv, nil
	}
}

func (r Runner) setIncusOSService(ctx context.Context, server provisioning.Server) func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var service string
		configArg := &starlark.Dict{}
		err := starlark.UnpackArgs(b.Name(), args, kwargs, "service", &service, "config", &configArg)
		if err != nil {
			return nil, err
		}

		config, err := scriptlet.StarlarkUnmarshal(configArg)
		if err != nil {
			return nil, err
		}

		err = r.client.UpdateOSService(ctx, server, service, config)
		if err != nil {
			return starlark.None, fmt.Errorf("Failed to set service %s configuration for server %q: %w", service, server.Name, err)
		}

		return starlark.None, nil
	}
}

func (r Runner) addIncusOSApplication(ctx context.Context, server provisioning.Server) func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var application string
		err := starlark.UnpackArgs(b.Name(), args, kwargs, "application", &application)
		if err != nil {
			return nil, err
		}

		err = r.client.AddApplication(ctx, server, application)
		if err != nil {
			return starlark.None, fmt.Errorf("Failed to add application %q to server %q: %w", application, server.Name, err)
		}

		return starlark.None, nil
	}
}
