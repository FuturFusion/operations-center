package supportbundle

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

// bundle writes the sections of a support bundle into a zip archive.
//
// A failing section is recorded in the manifest and replaced by an error file,
// only a failure to write the archive itself aborts it. This error is sticky,
// every later write is skipped and it is reported by close.
type bundle struct {
	zw       *zip.Writer
	root     string
	start    time.Time
	manifest manifest
	err      error
}

func newBundle(w io.Writer, root string, m manifest) *bundle {
	return &bundle{
		zw:       zip.NewWriter(w),
		root:     root,
		start:    time.Now(),
		manifest: m,
	}
}

// json writes the value returned by fn as redacted JSON to path.
func (b *bundle) json(ctx context.Context, name string, timeout time.Duration, fn func(ctx context.Context) (any, error)) {
	b.section(ctx, name, timeout, -1, func(ctx context.Context) (io.ReadCloser, error) {
		v, err := fn(ctx)
		if err != nil {
			return nil, err
		}

		content, err := redactJSON(v)
		if err != nil {
			return nil, err
		}

		return io.NopCloser(bytes.NewReader(content)), nil
	})
}

// text writes the string returned by fn as is to path.
func (b *bundle) text(ctx context.Context, name string, timeout time.Duration, fn func(ctx context.Context) (string, error)) {
	b.section(ctx, name, timeout, -1, func(ctx context.Context) (io.ReadCloser, error) {
		content, err := fn(ctx)
		if err != nil {
			return nil, err
		}

		return io.NopCloser(strings.NewReader(content)), nil
	})
}

// section copies the content returned by fn to path, it is truncated after limit bytes, if limit is not negative.
func (b *bundle) section(ctx context.Context, name string, timeout time.Duration, limit int64, fn func(ctx context.Context) (io.ReadCloser, error)) {
	if b.err != nil || !b.validPath(name) {
		return
	}

	start := time.Now()

	sectionCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	status, err := b.copySection(sectionCtx, name, limit, fn)
	if b.err != nil {
		return
	}

	if err != nil {
		status = sectionStatusError
		b.writeError(name, err)
	}

	b.record(name, status, err, time.Since(start))
}

func (b *bundle) copySection(ctx context.Context, name string, limit int64, fn func(ctx context.Context) (io.ReadCloser, error)) (sectionStatus, error) {
	rc, err := fn(ctx)
	if err != nil {
		return sectionStatusError, err
	}

	defer rc.Close()

	w, err := b.zw.Create(path.Join(b.root, name))
	if err != nil {
		b.err = err
		return sectionStatusError, err
	}

	if limit < 0 {
		_, err = io.Copy(w, &sourceReader{rc})
		return sectionStatusOK, b.copyErr(err)
	}

	n, err := io.Copy(w, &sourceReader{io.LimitReader(rc, limit)})
	if err != nil {
		return sectionStatusError, b.copyErr(err)
	}

	if n == limit {
		probe := make([]byte, 1)
		more, _ := rc.Read(probe)
		if more > 0 {
			return sectionStatusTruncated, nil
		}
	}

	return sectionStatusOK, nil
}

// sourceReader marks errors of the source, everything else is an error writing the archive.
type sourceReader struct {
	io.Reader
}

type sourceError struct {
	err error
}

func (e sourceError) Error() string {
	return e.err.Error()
}

func (e sourceError) Unwrap() error {
	return e.err
}

func (r *sourceReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, sourceError{err: err}
	}

	return n, err
}

// copyErr keeps a source error for the manifest and makes a write error fatal.
func (b *bundle) copyErr(err error) error {
	if err == nil {
		return nil
	}

	var srcErr sourceError
	if errors.As(err, &srcErr) {
		return srcErr.err
	}

	b.err = err

	return err
}

func (b *bundle) writeError(name string, sectionErr error) {
	w, err := b.zw.Create(path.Join(b.root, name+".error.txt"))
	if err != nil {
		b.err = err
		return
	}

	_, err = io.WriteString(w, sectionErr.Error()+"\n")
	if err != nil {
		b.err = err
	}
}

// skip records a section, which is not collected, together with the reason.
func (b *bundle) skip(name string, reason error) {
	if b.err != nil || !b.validPath(name) {
		return
	}

	b.writeError(name, reason)
	b.record(name, sectionStatusSkipped, reason, 0)
}

// validPath records a section, whose name leaves the root of the archive, as failed.
func (b *bundle) validPath(name string) bool {
	if strings.HasPrefix(path.Join(b.root, name), b.root+"/") {
		return true
	}

	b.record(name, sectionStatusError, errors.New("Invalid section path"), 0)

	return false
}

func (b *bundle) note(format string, a ...any) {
	b.manifest.Notes = append(b.manifest.Notes, fmt.Sprintf(format, a...))
}

func (b *bundle) record(name string, status sectionStatus, err error, duration time.Duration) {
	section := manifestSection{
		Path:       name,
		Status:     status,
		DurationMS: duration.Milliseconds(),
	}

	if err != nil {
		section.Error = err.Error()
	}

	b.manifest.Sections = append(b.manifest.Sections, section)
}

// close writes the manifest and completes the archive.
func (b *bundle) close() error {
	if b.err != nil {
		return b.err
	}

	b.manifest.DurationMS = time.Since(b.start).Milliseconds()

	content, err := json.MarshalIndent(b.manifest, "", "  ")
	if err != nil {
		return err
	}

	w, err := b.zw.Create(path.Join(b.root, "manifest.json"))
	if err != nil {
		return err
	}

	_, err = w.Write(content)
	if err != nil {
		return err
	}

	return b.zw.Close()
}
