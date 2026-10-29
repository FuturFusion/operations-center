package localrecording

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/version"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	manifestFilename  = "manifest.json"
	exchangesFilename = "exchanges.jsonl"
)

type bmcRecording struct {
	root *os.Root
	now  func() time.Time
}

var _ provisioning.BMCRecordingRepo = &bmcRecording{}

// Option configures the BMC recording repository.
type Option func(r *bmcRecording)

// WithNow sets the clock used for the end of a recording without a finish time.
func WithNow(now func() time.Time) Option {
	return func(r *bmcRecording) {
		r.now = now
	}
}

// New returns a BMC recording repository, which stores the recordings in root.
func New(root *os.Root, opts ...Option) *bmcRecording {
	r := &bmcRecording{
		root: root,
		now:  time.Now,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// serverDir returns the directory of the server relative to the root. The
// server name must be a single path element, so the path stays in the root.
func serverDir(serverName string) (string, error) {
	if !file.IsPathElement(serverName) {
		return "", domain.NewValidationErrf("Invalid server name %q for BMC recording", serverName)
	}

	return serverName, nil
}

// recordingDir returns the directory of the recording relative to the root.
func recordingDir(serverName string, id string) (string, error) {
	dir, err := serverDir(serverName)
	if err != nil {
		return "", err
	}

	err = provisioning.ValidateBMCRecordingID(id)
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, id), nil
}

func errRecordingNotFound(serverName string, id string) error {
	return domain.NewErrorf(domain.ErrNotFound, "", "BMC recording %q of server %q not found", id, serverName)
}

func (r bmcRecording) Open(ctx context.Context, server provisioning.Server, deployment provisioning.ServerDeployment) (provisioning.BMCRecordingSession, error) {
	id := provisioning.BMCRecordingID(deployment.StartedAt)

	dir, err := recordingDir(server.Name, id)
	if err != nil {
		return nil, err
	}

	err = r.root.MkdirAll(dir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("Failed to create directory for BMC recording %q: %w", dir, err)
	}

	manifest, err := r.readManifest(dir)
	if errors.Is(err, fs.ErrNotExist) {
		manifest = api.BMCRecordingManifest{
			FormatVersion:           api.BMCRecordingFormatVersion,
			Server:                  server.Name,
			RecordingID:             id,
			OperationsCenterVersion: version.Version,
			StartedAt:               deployment.StartedAt,
			Request:                 deployment.Request.ToAPI(),
			BMCData:                 server.BMCData,
			BMCEndpoint:             server.BMCConfig.Endpoint,
			MediaURLs:               mediaURLs(deployment),
		}

		err = writeManifest(r.root, dir, manifest)
	}

	if err != nil {
		return nil, err
	}

	exchangesPath := filepath.Join(dir, exchangesFilename)

	stats, err := recoverExchanges(r.root, exchangesPath)
	if err != nil {
		return nil, err
	}

	f, err := r.root.OpenFile(exchangesPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("Failed to open BMC recording exchanges %q: %w", exchangesPath, err)
	}

	r.prune(ctx, server.Name, id)

	return &session{
		root:     r.root,
		file:     f,
		dir:      dir,
		manifest: manifest,
		stats:    stats,
		now:      r.now,
	}, nil
}

// recoverExchanges continues the counts of an existing recording. A partial
// last line, left by a crash while writing, is cut off.
func recoverExchanges(root *os.Root, path string) (provisioning.BMCRecordingStats, error) {
	stats := provisioning.BMCRecordingStats{}

	f, err := root.OpenFile(path, os.O_RDWR, 0o600)
	if errors.Is(err, fs.ErrNotExist) {
		return stats, nil
	}

	if err != nil {
		return stats, fmt.Errorf("Failed to open BMC recording exchanges %q: %w", path, err)
	}

	defer f.Close()

	// A line has no upper bound, since JSON escaping can grow a body by a
	// large factor, so a bufio.Scanner with a fixed buffer is not used.
	reader := bufio.NewReader(f)

	var complete int64

	for {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return stats, fmt.Errorf("Failed to read BMC recording exchanges %q: %w", path, err)
		}

		var exchange struct {
			RequestTruncated  bool `json:"request_truncated"`
			ResponseTruncated bool `json:"response_truncated"`
		}

		err = json.Unmarshal(line, &exchange)
		if err != nil {
			break
		}

		complete += int64(len(line))
		stats.Exchanges++

		if exchange.RequestTruncated || exchange.ResponseTruncated {
			stats.TruncatedExchanges++
		}
	}

	stats.Bytes = complete

	err = f.Truncate(complete)
	if err != nil {
		return stats, fmt.Errorf("Failed to cut partial line from BMC recording exchanges %q: %w", path, err)
	}

	return stats, nil
}

// prune removes the oldest recordings of the server beyond the limit. The
// recording just opened is kept in any case.
func (r bmcRecording) prune(ctx context.Context, serverName string, keepID string) {
	ids, err := r.recordingIDs(serverName)
	if err != nil {
		slog.WarnContext(ctx, "Failed to list BMC recordings for pruning", slog.String("server", serverName), logger.Err(err))
		return
	}

	excess := len(ids) - config.BMCRecordingsPerServer
	for _, id := range ids {
		if excess <= 0 {
			break
		}

		if id == keepID {
			continue
		}

		err = r.root.RemoveAll(filepath.Join(serverName, id))
		if err != nil {
			slog.WarnContext(ctx, "Failed to prune BMC recording", slog.String("server", serverName), slog.String("id", id), logger.Err(err))
		}

		excess--
	}
}

// recordingIDs returns the IDs of the recordings of the server, oldest first.
func (r bmcRecording) recordingIDs(serverName string) ([]string, error) {
	dir, err := serverDir(serverName)
	if err != nil {
		return nil, err
	}

	entries, err := fs.ReadDir(r.root.FS(), dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("Failed to read BMC recordings of server %q: %w", serverName, err)
	}

	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || provisioning.ValidateBMCRecordingID(entry.Name()) != nil {
			continue
		}

		ids = append(ids, entry.Name())
	}

	// The ID is a UTC time stamp, so the order by name is the order by time.
	slices.Sort(ids)

	return ids, nil
}

func (r bmcRecording) GetAll(ctx context.Context, serverName string) ([]api.BMCRecordingManifest, error) {
	ids, err := r.recordingIDs(serverName)
	if err != nil {
		return nil, err
	}

	manifests := make([]api.BMCRecordingManifest, 0, len(ids))
	for _, id := range slices.Backward(ids) {
		manifest, err := r.readManifest(filepath.Join(serverName, id))
		if err != nil {
			slog.WarnContext(ctx, "Skipping BMC recording without readable manifest", slog.String("server", serverName), slog.String("id", id), logger.Err(err))
			continue
		}

		manifests = append(manifests, manifest)
	}

	return manifests, nil
}

func (r bmcRecording) GetByID(ctx context.Context, serverName string, id string) (*api.BMCRecordingManifest, error) {
	dir, err := recordingDir(serverName, id)
	if err != nil {
		return nil, err
	}

	manifest, err := r.readManifest(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errRecordingNotFound(serverName, id)
	}

	if err != nil {
		return nil, err
	}

	return &manifest, nil
}

func (r bmcRecording) GetFile(ctx context.Context, serverName string, id string, filename string) (_ io.ReadCloser, size int64, _ error) {
	dir, err := recordingDir(serverName, id)
	if err != nil {
		return nil, 0, err
	}

	if filename != manifestFilename && filename != exchangesFilename {
		return nil, 0, domain.NewValidationErrf("Invalid BMC recording file %q, allowed are %q and %q", filename, manifestFilename, exchangesFilename)
	}

	f, err := r.root.Open(filepath.Join(dir, filename))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, errRecordingNotFound(serverName, id)
	}

	if err != nil {
		return nil, 0, fmt.Errorf("Failed to open BMC recording file %q: %w", filename, err)
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, fmt.Errorf("Failed to get size of BMC recording file %q: %w", filename, err)
	}

	return f, info.Size(), nil
}

func (r bmcRecording) GetArchive(ctx context.Context, serverName string, id string, rewrite *provisioning.BMCRecordingRewrite) (_ io.ReadCloser, size int, _ error) {
	dir, err := recordingDir(serverName, id)
	if err != nil {
		return nil, 0, err
	}

	if !file.PathExistsIn(r.root, dir) {
		return nil, 0, errRecordingNotFound(serverName, id)
	}

	// The body limit and the retention limit bound the size, so the archive is
	// built in memory.
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	for _, filename := range []string{manifestFilename, exchangesFilename} {
		err = addToArchive(zipWriter, r.root, filepath.Join(dir, filename), rewrite)
		if err != nil {
			return nil, 0, err
		}
	}

	err = zipWriter.Close()
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to close zip archive: %w", err)
	}

	return io.NopCloser(buf), buf.Len(), nil
}

func addToArchive(zipWriter *zip.Writer, root *os.Root, path string, rewrite *provisioning.BMCRecordingRewrite) error {
	source, err := root.Open(path)
	if err != nil {
		return fmt.Errorf("Failed to open BMC recording file %q: %w", path, err)
	}

	defer source.Close()

	target, err := zipWriter.Create(filepath.Base(path))
	if err != nil {
		return fmt.Errorf("Failed to create %q in zip file: %w", filepath.Base(path), err)
	}

	if rewrite == nil {
		_, err = file.SafeCopy(target, source)
		if err != nil {
			return fmt.Errorf("Failed to copy BMC recording file %q to zip archive: %w", path, err)
		}

		return nil
	}

	reader := bufio.NewReader(source)
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fmt.Errorf("Failed to read BMC recording file %q: %w", path, readErr)
		}

		content, newline := line, ""
		if len(line) > 0 && line[len(line)-1] == '\n' {
			content, newline = line[:len(line)-1], "\n"
		}

		_, err = io.WriteString(target, rewrite.Apply(content)+newline)
		if err != nil {
			return fmt.Errorf("Failed to write BMC recording file %q to zip archive: %w", path, err)
		}

		if readErr != nil {
			return nil
		}
	}
}

func (r bmcRecording) DeleteByID(ctx context.Context, serverName string, id string) error {
	dir, err := recordingDir(serverName, id)
	if err != nil {
		return err
	}

	err = r.root.RemoveAll(dir)
	if err != nil {
		return fmt.Errorf("Failed to delete BMC recording %q of server %q: %w", id, serverName, err)
	}

	return nil
}

func (r bmcRecording) DeleteAll(ctx context.Context, serverName string) error {
	dir, err := serverDir(serverName)
	if err != nil {
		return err
	}

	err = r.root.RemoveAll(dir)
	if err != nil {
		return fmt.Errorf("Failed to delete BMC recordings of server %q: %w", serverName, err)
	}

	return nil
}

func (r bmcRecording) readManifest(dir string) (api.BMCRecordingManifest, error) {
	var manifest api.BMCRecordingManifest

	content, err := r.root.ReadFile(filepath.Join(dir, manifestFilename))
	if err != nil {
		return manifest, fmt.Errorf("Failed to read BMC recording manifest: %w", err)
	}

	err = json.Unmarshal(content, &manifest)
	if err != nil {
		return manifest, fmt.Errorf("Failed to parse BMC recording manifest: %w", err)
	}

	return manifest, nil
}

func writeManifest(root *os.Root, dir string, manifest api.BMCRecordingManifest) error {
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to encode BMC recording manifest: %w", err)
	}

	err = file.WriteFileAtomicIn(root, filepath.Join(dir, manifestFilename), append(content, '\n'), 0o600)
	if err != nil {
		return fmt.Errorf("Failed to write BMC recording manifest: %w", err)
	}

	return nil
}

func mediaURLs(deployment provisioning.ServerDeployment) []string {
	var urls []string
	for _, url := range []string{deployment.MediaURL, deployment.SecureBootMediaURL} {
		if url != "" {
			urls = append(urls, url)
		}
	}

	return urls
}
