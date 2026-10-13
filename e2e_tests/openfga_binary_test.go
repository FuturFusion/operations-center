package e2e

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	openFGAVersion = "1.8.9"

	openFGABinaryName = "openfga"

	// openFGABinaryMaxSize protects the disk against a tarball that expands to
	// a huge file.
	openFGABinaryMaxSize = 512 * 1024 * 1024
)

// openFGATarballSHA256 holds the SHA-256 sums of the release tarballs of
// openFGAVersion. The sums are from the checksums.txt file of the release.
// Update the sums together with openFGAVersion.
var openFGATarballSHA256 = map[string]string{
	"linux_amd64": "a3123260754323b7c592f8218179feccdee0e12851e241cd4fe53b509dc4bf5b",
	"linux_arm64": "0978aa24057f1183c4a0a11a24f31c4f89c26ee95144d51ab7c07301563ea5af",
}

// ensureOpenFGABinary returns the path of the OpenFGA binary below tmpDir. It
// downloads the binary, if it is missing or if it has the wrong version.
func ensureOpenFGABinary(t *testing.T, tmpDir string) string {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(5*time.Minute))
	defer cancel()

	binary := filepath.Join(tmpDir, "openfga", openFGABinaryName)

	err := checkOpenFGABinaryVersion(ctx, binary)
	if err == nil {
		return binary
	}

	t.Logf("Download OpenFGA v%s: %v", openFGAVersion, err)

	err = downloadOpenFGABinary(ctx, binary)
	require.NoError(t, err, "Failed to download the OpenFGA binary")

	err = checkOpenFGABinaryVersion(ctx, binary)
	require.NoError(t, err, "Failed to verify the downloaded OpenFGA binary")

	return binary
}

// checkOpenFGABinaryVersion returns an error, if binary is missing or if it
// does not report openFGAVersion.
func checkOpenFGABinaryVersion(ctx context.Context, binary string) error {
	_, err := os.Stat(binary)
	if err != nil {
		return err
	}

	out, err := exec.CommandContext(ctx, binary, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run %q: %w: %s", binary+" version", err, strings.TrimSpace(string(out)))
	}

	// The output has the form "OpenFGA version `v<version>` build from ...".
	if !strings.Contains(string(out), "`v"+openFGAVersion+"`") {
		return fmt.Errorf("%q is not version v%s: %s", binary, openFGAVersion, strings.TrimSpace(string(out)))
	}

	return nil
}

// downloadOpenFGABinary downloads the release tarball of openFGAVersion,
// verifies its SHA-256 sum and extracts the binary to target.
func downloadOpenFGABinary(ctx context.Context, target string) (err error) {
	platform := runtime.GOOS + "_" + runtime.GOARCH

	wantSum, ok := openFGATarballSHA256[platform]
	if !ok {
		return fmt.Errorf("no SHA-256 sum for OpenFGA v%s on %s", openFGAVersion, platform)
	}

	targetDir := filepath.Dir(target)

	err = os.MkdirAll(targetDir, 0o700)
	if err != nil {
		return err
	}

	tarball, err := os.CreateTemp(targetDir, "openfga-*.tar.gz")
	if err != nil {
		return err
	}

	defer func() {
		err = errors.Join(err, tarball.Close(), os.Remove(tarball.Name()))
	}()

	url := fmt.Sprintf("https://github.com/openfga/openfga/releases/download/v%[1]s/openfga_%[1]s_%[2]s.tar.gz", openFGAVersion, platform)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %q for %s", resp.Status, url)
	}

	hash := sha256.New()

	_, err = io.Copy(io.MultiWriter(tarball, hash), resp.Body)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}

	gotSum := hex.EncodeToString(hash.Sum(nil))
	if gotSum != wantSum {
		return fmt.Errorf("SHA-256 sum mismatch for %s: got %s, want %s", url, gotSum, wantSum)
	}

	_, err = tarball.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	return extractOpenFGABinary(tarball, target)
}

func extractOpenFGABinary(tarball io.Reader, target string) (err error) {
	gzipReader, err := gzip.NewReader(tarball)
	if err != nil {
		return err
	}

	defer func() {
		err = errors.Join(err, gzipReader.Close())
	}()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("no %q in the OpenFGA release tarball", openFGABinaryName)
		}

		if err != nil {
			return err
		}

		if header.Typeflag == tar.TypeReg && header.Name == openFGABinaryName {
			break
		}
	}

	// Write to a temporary file and rename it. Linux cannot overwrite a running
	// binary.
	binary, err := os.CreateTemp(filepath.Dir(target), openFGABinaryName+"-*")
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(binary.Name()))
		}
	}()

	_, err = io.Copy(binary, io.LimitReader(tarReader, openFGABinaryMaxSize))
	if err != nil {
		return errors.Join(err, binary.Close())
	}

	err = binary.Close()
	if err != nil {
		return err
	}

	err = os.Chmod(binary.Name(), 0o700)
	if err != nil {
		return err
	}

	return os.Rename(binary.Name(), target)
}
