package localrecording

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

type session struct {
	mu       sync.Mutex
	root     *os.Root
	file     *os.File
	dir      string
	manifest api.BMCRecordingManifest
	stats    provisioning.BMCRecordingStats
	closed   bool
	now      func() time.Time
}

var _ provisioning.BMCRecordingSession = &session{}

// Record appends the exchange. Errors are logged and dropped, since a
// recording must never fail a deployment.
func (s *session) Record(ctx context.Context, exchange api.BMCRecordingExchange) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		slog.DebugContext(ctx, "Dropping BMC exchange for finalized recording", slog.String("dir", s.dir))
		return
	}

	exchange.Sequence = s.stats.Exchanges + 1

	line, err := json.Marshal(exchange)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to encode BMC exchange", slog.String("dir", s.dir), logger.Err(err))
		return
	}

	line = append(line, '\n')

	_, err = s.file.Write(line)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to write BMC exchange", slog.String("dir", s.dir), logger.Err(err))
		return
	}

	s.stats.Exchanges++
	s.stats.Bytes += int64(len(line))

	if exchange.RequestTruncated || exchange.ResponseTruncated {
		s.stats.TruncatedExchanges++
	}
}

func (s *session) Stats() provisioning.BMCRecordingStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.stats
}

func (s *session) Finalize(ctx context.Context, deployment provisioning.ServerDeployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	manifest := s.manifest
	manifest.FinishedAt = deployment.FinishedAt
	if manifest.FinishedAt.IsZero() {
		manifest.FinishedAt = s.now()
	}

	manifest.FinalState = deployment.State
	manifest.History = deployment.History
	manifest.MediaURLs = mediaURLs(deployment)
	manifest.Exchanges = s.stats.Exchanges
	manifest.Bytes = s.stats.Bytes
	manifest.TruncatedExchanges = s.stats.TruncatedExchanges

	s.manifest = manifest

	writeErr := writeManifest(s.root, s.dir, manifest)

	closeErr := s.file.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("Failed to close BMC recording exchanges: %w", closeErr)
	}

	return errors.Join(writeErr, closeErr)
}
