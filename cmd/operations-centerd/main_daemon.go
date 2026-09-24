package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/lxc/incus/v7/shared/util"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	restapi "github.com/FuturFusion/operations-center/internal/api"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/system"
	systemLocalfs "github.com/FuturFusion/operations-center/internal/system/repo/localfs"
	"github.com/FuturFusion/operations-center/internal/util/logger"
)

var componentDaemon = logger.RegisterComponent("daemon")

type env interface {
	LogDir() string
	RunDir() string
	VarDir() string
	CacheDir() string
	UsrShareDir() string
	GetUnixSocket() string
	IsIncusOS() bool
	GetToken(ctx context.Context) (string, error)
	GetSecureBootCertificates(ctx context.Context) (incusosapi.InternalSecureBootCertificates, error)
	GetSecureBootPlatformKeyUpdate(ctx context.Context) ([]byte, error)
}

type cmdDaemon struct {
	env env
}

func (c *cmdDaemon) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = config.BinaryName
	cmd.Short = "The operations center daemon"
	cmd.Long = `Description:
  The operations center daemon

  This is the operations center daemon command line.
`
	cmd.RunE = c.Run

	return cmd
}

func (c *cmdDaemon) Run(cmd *cobra.Command, args []string) error {
	if len(args) > 1 || (len(args) == 1 && args[0] != config.BinaryName && args[0] != "") {
		return fmt.Errorf(`Unknown command "%s" for "%s"`, args[0], cmd.CommandPath())
	}

	// Ensure we have the data directory.
	err := os.MkdirAll(c.env.VarDir(), 0o750)
	if err != nil {
		return fmt.Errorf("Create data directory %q: %v", c.env.VarDir(), err)
	}

	logCtx := logger.ContextWithComponent(cmd.Context(), componentDaemon)

	err = systemLocalfs.NewBackup(c.env.VarDir()).ApplyRestore()
	if err != nil {
		if !errors.Is(err, system.ErrRestoreRolledBack) {
			return fmt.Errorf("Failed to prepare data directory %q: %w", c.env.VarDir(), err)
		}

		slog.ErrorContext(logCtx, "Failed to restore from backup", logger.Err(err))
	}

	// Ensure we have the run directory.
	err = os.MkdirAll(c.env.RunDir(), 0o750)
	if err != nil {
		return fmt.Errorf("Create run directory %q: %v", c.env.RunDir(), err)
	}

	err = config.Init(c.env)
	if err != nil {
		return fmt.Errorf("Failed to load config from %q: %w", c.env.VarDir(), err)
	}

	if !cmd.Flag("verbose").Changed && !cmd.Flag("debug").Changed {
		err = logger.SetLogLevel(logger.ParseLevel(config.GetSettings().LogLevel))
		if err != nil {
			return fmt.Errorf("Failed to set log level to %q from config: %w", config.GetSettings().LogLevel, err)
		}
	}

	err = logger.SetComponentLevels(logger.ParseComponentLevels(config.GetSettings().LogLevels))
	if err != nil {
		return fmt.Errorf("Failed to set per component log levels from config: %w", err)
	}

	rootCtx, stop := signal.NotifyContext(
		context.Background(),
		unix.SIGPWR,
		unix.SIGINT,
		unix.SIGQUIT,
		unix.SIGTERM,
	)
	defer stop()

	// Generate client certificate if none are found.
	clientCertFilename := filepath.Join(c.env.VarDir(), config.ClientCertificateFilename)
	clientKeyFilename := filepath.Join(c.env.VarDir(), config.ClientKeyFilename)
	if !util.PathExists(clientCertFilename) || !util.PathExists(clientKeyFilename) {
		slog.InfoContext(logCtx, "No client certificate found, generate client.crt and client.key")
		err := incustls.FindOrGenCert(clientCertFilename, clientKeyFilename, true, false)
		if err != nil {
			return fmt.Errorf("Failed to generate client certificate: %w", err)
		}
	}

	d := restapi.NewDaemon(cmd.Context(), c.env)

	err = d.Start(cmd.Context())
	if err != nil {
		slog.ErrorContext(logCtx, "Failed to start daemon", logger.Err(err))
		return fmt.Errorf("Failed to start daemon: %v", err)
	}

	slog.InfoContext(logCtx, "Daemon started")

	restart := false
	select {
	case <-rootCtx.Done():
	case <-d.RestartRequested():
		restart = true
	}

	slog.InfoContext(logCtx, "Shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	stopErr := d.Stop(shutdownCtx)
	if stopErr != nil {
		slog.ErrorContext(logCtx, "Error occurred during shutdown of daemon", logger.Err(stopErr))
	} else {
		slog.InfoContext(logCtx, "Daemon shutdown completed successfully")
	}

	// A requested restart is performed even if the shutdown failed.
	if restart {
		err = restartDaemon(logCtx)
		if err != nil {
			slog.ErrorContext(logCtx, "Failed to restart daemon", logger.Err(err))
			return fmt.Errorf("Failed to restart daemon: %v", err)
		}
	}

	if stopErr != nil {
		return fmt.Errorf("Error occurred during shutdown of daemon: %v", stopErr)
	}

	return nil
}

// restartDaemon replaces the running process with a new instance of the daemon.
func restartDaemon(ctx context.Context) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("Failed to determine executable: %w", err)
	}

	slog.InfoContext(ctx, "Restarting daemon")

	err = unix.Exec(executable, os.Args, os.Environ())
	if err != nil {
		return fmt.Errorf("Failed to execute %q: %w", executable, err)
	}

	return nil
}
