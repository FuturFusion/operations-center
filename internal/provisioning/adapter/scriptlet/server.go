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
		scriptlet.Required(serverRegistration): {"server"},
	})
	if err != nil {
		return fmt.Errorf("Failed to validate server registration scriptlet: %w", err)
	}

	return nil
}

// serverRegistrationCompile compiles the server registration scriptlet.
func serverRegistrationCompile(name string, src string) (*starlark.Program, error) {
	return scriptlet.Compile(name, src, []string{
		"log_info",
		"log_warn",
		"log_error",

		"set_server_name",
		"set_server_description",
		"set_server_properties",
		"set_server_connection_url",
		"set_server_update_channel",

		"get_system",
		"set_system",
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

	env := starlark.StringDict{
		"log_info":  starlark.NewBuiltin("log_info", logFunc),
		"log_warn":  starlark.NewBuiltin("log_warn", logFunc),
		"log_error": starlark.NewBuiltin("log_error", logFunc),

		"set_server_name":           starlark.NewBuiltin("set_server_name", setServerName(ctx, server)),
		"set_server_description":    starlark.NewBuiltin("set_server_description", setServerDescription(ctx, server)),
		"set_server_properties":     starlark.NewBuiltin("set_server_properties", setServerProperties(ctx, server)),
		"set_server_connection_url": starlark.NewBuiltin("set_server_connection_url", setServerConnectionURL(ctx, server)),
		"set_server_update_channel": starlark.NewBuiltin("set_server_update_channel", setServerUpdateChannel(ctx, server)),

		"get_system": starlark.NewBuiltin("get_system", r.getSystem(ctx, *server)),
		"set_system": starlark.NewBuiltin("set_system", r.setSystem(ctx, *server)),
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
		{starlark.String("server"), serverv},
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

func (r Runner) getSystem(ctx context.Context, server provisioning.Server) func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

func (r Runner) setSystem(ctx context.Context, server provisioning.Server) func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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
