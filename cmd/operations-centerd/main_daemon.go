package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/lxc/incus/v6/shared/util"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	restapi "github.com/FuturFusion/operations-center/internal/api"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/logger"
)

const defaultRestServerPort = 7443

type env interface {
	LogDir() string
	RunDir() string
	VarDir() string
	UsrShareDir() string
	GetUnixSocket() string
}

type cmdDaemon struct {
	env env

	flagServerAddr string
	flagServerPort int
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

	cmd.Flags().StringVar(&c.flagServerAddr, "server-addr", "", "Address to bind to")
	cmd.Flags().IntVar(&c.flagServerPort, "server-port", defaultRestServerPort, "IP port to bind to")

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

	// Ensure we have the run directory.
	err = os.MkdirAll(c.env.RunDir(), 0o750)
	if err != nil {
		return fmt.Errorf("Create run directory %q: %v", c.env.RunDir(), err)
	}

	err = config.Init(c.env)
	if err != nil {
		return fmt.Errorf("Failed to load config from %q: %w", c.env.VarDir(), err)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(),
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
		slog.InfoContext(cmd.Context(), "No client certificate found, generate client.crt and client.key")
		err := incustls.FindOrGenCert(clientCertFilename, clientKeyFilename, true, false)
		if err != nil {
			return fmt.Errorf("Failed to generate client certificate: %w", err)
		}
	}

	d := restapi.NewDaemon(cmd.Context(), c.env)

	err = d.Start(cmd.Context())
	if err != nil {
		slog.ErrorContext(cmd.Context(), "Failed to start daemon", logger.Err(err))
		return fmt.Errorf("Failed to start daemon: %v", err)
	}

	slog.InfoContext(cmd.Context(), "Daemon started")

	<-rootCtx.Done()
	slog.InfoContext(cmd.Context(), "Shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	err = d.Stop(shutdownCtx)
	if err != nil {
		slog.ErrorContext(cmd.Context(), "Error occurred during shutdown of daemon", logger.Err(err))
		return fmt.Errorf("Error occurred during shutdown of daemon: %v", err)
	}

	slog.InfoContext(cmd.Context(), "Daemon shutdown completed successfully")

	return nil
}
