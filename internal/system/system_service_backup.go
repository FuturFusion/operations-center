package system

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"

	"github.com/lxc/incus/v7/shared/revert"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

// ErrRestoreRolledBack reports a failed restore, which has been rolled back.
var ErrRestoreRolledBack = errors.New("Restore did not complete, the state before the restore has been put back")

// BackupOptions selects the optional content of a backup.
type BackupOptions struct {
	Images  bool
	Updates bool
}

func (s *systemService) Backup(ctx context.Context, options BackupOptions) (io.ReadCloser, error) {
	return s.backupRepo.Create(ctx, options, func(ctx context.Context, dir string) error {
		return s.databaseRepo.Snapshot(ctx, dir, options)
	})
}

func (s *systemService) Restore(ctx context.Context, archive io.Reader) error {
	if !s.restoreMu.TryLock() {
		return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "A restore is already in progress")
	}

	reverter := revert.New()
	defer reverter.Fail()

	reverter.Add(s.restoreMu.Unlock)

	err := s.checkNoOperationInProgress(ctx)
	if err != nil {
		return err
	}

	dir, err := s.backupRepo.Extract(ctx, archive)
	if err != nil {
		return err
	}

	reverter.Add(func() {
		err := s.backupRepo.Discard(ctx)
		if err != nil {
			slog.WarnContext(ctx, "Failed to discard backup after unsuccessful restore", logger.Err(err))
		}
	})

	err = s.validateBackup(ctx, dir)
	if err != nil {
		return err
	}

	err = s.backupRepo.Stage(ctx)
	if err != nil {
		return err
	}

	reverter.Success()

	// The staged restore is applied by the restarted daemon.
	s.requestRestart()

	return nil
}

// checkNoOperationInProgress prevents a restore, while an operation is in progress.
func (s *systemService) checkNoOperationInProgress(ctx context.Context) error {
	servers, err := s.serverSvc.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("Failed to get servers: %w", err)
	}

	for _, server := range servers {
		if server.StatusInternal.Deployment.IsActive() {
			return serverOperationInProgressErr(server.Name, "being deployed")
		}

		switch server.StatusDetail {
		case api.ServerStatusDetailReadyUpdatingOS,
			api.ServerStatusDetailReadyUpdatingApplication,
			api.ServerStatusDetailReadyEvacuating,
			api.ServerStatusDetailReadyRestoring:
			return serverOperationInProgressErr(server.Name, string(server.StatusDetail))
		}
	}

	clusters, err := s.clusterSvc.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("Failed to get clusters: %w", err)
	}

	for _, cluster := range clusters {
		inProgress := cluster.UpdateStatus.InProgressStatus.InProgress
		if inProgress != api.ClusterUpdateInProgressInactive && inProgress != api.ClusterUpdateInProgressError {
			return clusterOperationInProgressErr(cluster.Name, string(inProgress))
		}
	}

	return nil
}

func serverOperationInProgressErr(name string, operation string) error {
	return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Backup can not be restored while server %q is %s", name, operation).
		WithHintf("Wait for the operation to complete or abort it first.").
		WithDetail("server", name)
}

func clusterOperationInProgressErr(name string, operation string) error {
	return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Backup can not be restored while cluster %q is %s", name, operation).
		WithHintf("Wait for the operation to complete or abort it first.").
		WithDetail("cluster", name)
}

// validateBackup verifies, that Operations Center can start with the backup in dir.
func (s *systemService) validateBackup(ctx context.Context, dir string) error {
	_, err := tls.LoadX509KeyPair(filepath.Join(dir, config.ServerCertificateFilename), filepath.Join(dir, config.ServerKeyFilename))
	if err != nil {
		return invalidBackupErr(err, "Backup contains an invalid server certificate")
	}

	_, err = tls.LoadX509KeyPair(filepath.Join(dir, config.ClientCertificateFilename), filepath.Join(dir, config.ClientKeyFilename))
	if err != nil {
		return invalidBackupErr(err, "Backup contains an invalid client certificate")
	}

	err = config.ValidateFile(backupEnv{varDir: dir, isIncusOS: s.env.IsIncusOS()})
	if err != nil {
		return invalidBackupErr(err, "Backup contains an invalid configuration")
	}

	current, latest, err := s.databaseRepo.SchemaVersion(ctx, dir)
	if err != nil || current == 0 {
		return invalidBackupErr(err, "Backup contains an invalid database")
	}

	if current > latest {
		return domain.NewErrorf(domain.ErrInvalidArgument, "", "Backup has been created by a newer version of Operations Center").
			WithHintf("Update Operations Center before restoring this backup.")
	}

	return nil
}

// backupEnv presents an extracted backup as var dir for the config validation.
type backupEnv struct {
	varDir    string
	isIncusOS bool
}

func (e backupEnv) VarDir() string {
	return e.varDir
}

func (e backupEnv) IsIncusOS() bool {
	return e.isIncusOS
}

// invalidBackupErr reports a backup, which can not be restored.
func invalidBackupErr(cause error, message string) error {
	return domain.NewErrorf(domain.ErrInvalidArgument, "", "%s", message).
		WithCause(cause).
		WithHintf("Provide a backup created by Operations Center.")
}
