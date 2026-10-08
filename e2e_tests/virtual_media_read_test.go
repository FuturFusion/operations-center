package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	// Offset of the seed tarball in the installation media. It has to match
	// seedTarballStartPosition of the flasher.
	deployMediaSeedOffset int64 = 2148532224

	deployMediaSequentialReadSize int64 = 1 << 20
	deployMediaRandomReadSize     int64 = 64 << 10
)

type mediaReadTest struct {
	name        string
	rangeHeader string

	wantFrom   int64
	wantTo     int64
	assertBody func(t *testing.T, body []byte)
}

// assertVirtualMediaRandomRead reads the installation media of the deployment
// the way a BMC streaming it does: the start sequentially, then the end, then
// random portions.
func assertVirtualMediaRandomRead(t *testing.T, name string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	// Setup
	mediaURL := mustRun(t, `../bin/operations-center.linux.%s provisioning server deploy-status %s -f json | jq -r -e '.media_url'`, cpuArch, name).OutputTrimmed()

	// No client certificate is presented, since a BMC has none either.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // nolint: gosec // self signed certificate of the Operations Center under test.
		},
	}

	header, _ := mustRequestMedia(t, client, http.MethodHead, mediaURL, http.Header{}, http.StatusOK)
	require.Equal(t, "bytes", header.Get("Accept-Ranges"), "expect the installation media to be served with support for byte ranges")

	size, err := strconv.ParseInt(header.Get("Content-Length"), 10, 64)
	require.NoError(t, err, "expect the installation media to be served with its size")
	require.Greater(t, size, deployMediaSeedOffset+deployMediaSequentialReadSize, "expect the installation media to contain the seed tarball")

	noBodyAssertion := func(t *testing.T, body []byte) {
		t.Helper()
	}

	var tail []byte

	randomSeed := uint64(time.Now().UnixNano())
	random := rand.New(rand.NewPCG(randomSeed, 0)) // nolint: gosec // offsets to read at, reproducible by the logged seed.

	t.Logf("Random offsets are derived from seed %d", randomSeed)

	randomRead := func(name string) mediaReadTest {
		offset := random.Int64N(size - deployMediaRandomReadSize)

		return mediaReadTest{
			name:        name,
			rangeHeader: fmt.Sprintf("bytes=%d-%d", offset, offset+deployMediaRandomReadSize-1),

			wantFrom:   offset,
			wantTo:     offset + deployMediaRandomReadSize - 1,
			assertBody: noBodyAssertion,
		}
	}

	tests := []mediaReadTest{
		{
			name:        "start of the image",
			rangeHeader: fmt.Sprintf("bytes=0-%d", deployMediaSequentialReadSize-1),

			wantFrom:   0,
			wantTo:     deployMediaSequentialReadSize - 1,
			assertBody: noBodyAssertion,
		},
		{
			name:        "sequential read after the start of the image",
			rangeHeader: fmt.Sprintf("bytes=%d-%d", deployMediaSequentialReadSize, 2*deployMediaSequentialReadSize-1),

			wantFrom:   deployMediaSequentialReadSize,
			wantTo:     2*deployMediaSequentialReadSize - 1,
			assertBody: noBodyAssertion,
		},
		{
			name:        "end of the image as open ended range",
			rangeHeader: fmt.Sprintf("bytes=%d-", size-deployMediaSequentialReadSize),

			wantFrom: size - deployMediaSequentialReadSize,
			wantTo:   size - 1,
			assertBody: func(t *testing.T, body []byte) {
				t.Helper()

				tail = body
			},
		},
		{
			name:        "end of the image as suffix range",
			rangeHeader: fmt.Sprintf("bytes=-%d", deployMediaSequentialReadSize),

			wantFrom: size - deployMediaSequentialReadSize,
			wantTo:   size - 1,
			assertBody: func(t *testing.T, body []byte) {
				t.Helper()

				require.True(t, bytes.Equal(tail, body), "expect both forms of requesting the end of the image to return the same content")
			},
		},
		{
			name:        "seed tarball",
			rangeHeader: fmt.Sprintf("bytes=%d-%d", deployMediaSeedOffset, deployMediaSeedOffset+511),

			wantFrom: deployMediaSeedOffset,
			wantTo:   deployMediaSeedOffset + 511,
			assertBody: func(t *testing.T, body []byte) {
				t.Helper()

				require.Equal(t, "ustar", string(body[257:262]), "expect the header of the seed tarball at its offset in the image")
			},
		},
		randomRead("first random portion"),
		randomRead("second random portion"),
		randomRead("third random portion"),
	}

	for _, tc := range tests {
		t.Logf("Read %s with range %q", tc.name, tc.rangeHeader)

		// Run test
		header, body := mustRequestMedia(t, client, http.MethodGet, mediaURL, http.Header{"Range": []string{tc.rangeHeader}}, http.StatusPartialContent)

		// Assertions
		require.Equal(t, fmt.Sprintf("bytes %d-%d/%d", tc.wantFrom, tc.wantTo, size), header.Get("Content-Range"), "expect the requested range to be served for %s", tc.name)
		require.Len(t, body, int(tc.wantTo-tc.wantFrom+1), "expect the complete range to be served for %s", tc.name)
		tc.assertBody(t, body)
	}

	// Run test
	header, _ = mustRequestMedia(t, client, http.MethodGet, mediaURL, http.Header{"Range": []string{fmt.Sprintf("bytes=%d-", size)}}, http.StatusRequestedRangeNotSatisfiable)

	// Assertions
	require.Equal(t, fmt.Sprintf("bytes */%d", size), header.Get("Content-Range"), "expect a range past the end of the image to be answered with the size of the image")
}

// mustRequestMedia requests the installation media and returns the header and
// the body of the response.
func mustRequestMedia(t *testing.T, client *http.Client, method string, mediaURL string, header http.Header, wantStatus int) (http.Header, []byte) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(time.Minute))
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, mediaURL, nil)
	require.NoError(t, err)

	req.Header = header

	resp, err := client.Do(req)
	require.NoErrorf(t, err, "Failed to request the installation media %q", mediaURL)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	err = resp.Body.Close()
	require.NoError(t, err)

	require.Equalf(t, wantStatus, resp.StatusCode, "expect status %d for %s %q with header %v", wantStatus, method, mediaURL, header)

	return resp.Header, body
}
