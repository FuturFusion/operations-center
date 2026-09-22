package e2e

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Prevent functions from being seen as unused.
var _ = runWithTimeout

func Test_cmd(t *testing.T) {
	tests := []struct {
		name string
		cmd  string

		wantSuccess bool
		wantOutput  string
	}{
		{
			name: "true",
			cmd:  "true",

			wantSuccess: true,
		},
		{
			name: "false",
			cmd:  "false",

			wantSuccess: false,
		},
		{
			name: "pipefail",
			cmd:  "false | true",

			wantSuccess: false,
		},
		{
			name: "echo with arguments",
			cmd:  "echo -n foo bar baz",

			wantSuccess: true,
			wantOutput:  "foo bar baz",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := run(t, `%s`, tc.cmd)
			require.NoError(t, resp.err)

			require.Equal(t, tc.wantSuccess, resp.Success())
			require.Equal(t, tc.wantOutput, resp.Output())
		})
	}
}

func Test_isTransientStorageError(t *testing.T) {
	tests := []struct {
		name   string
		output string

		want bool
	}{
		{
			name:   "empty",
			output: "",

			want: false,
		},
		{
			name:   "failed to deactivate zvol",
			output: "Error: Failed unmounting instance: Failed to deactivate zvol after 5m0s\nTry `incus info --show-log IncusOS02` for more info\n",

			want: true,
		},
		{
			name:   "failed to activate volume",
			output: "Error: Failed to activate volume: Failed to locate zvol for default/virtual-machines/IncusOS01\n",

			want: true,
		},
		{
			name:   "dataset is busy",
			output: "Error: Failed to delete the instance: cannot destroy 'default/virtual-machines/IncusOS01': dataset is busy\n",

			want: true,
		},
		{
			name:   "device or resource busy",
			output: "Error: Failed to unmount: umount: /var/lib/incus/storage-pools/default: device or resource busy\n",

			want: true,
		},
		{
			name:   "instance not found",
			output: "Error: Instance not found\n",

			want: false,
		},
		{
			name:   "instance is already stopped",
			output: "Error: The instance is already stopped\n",

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := cmdResponse{
				output: bytes.NewBufferString(tc.output),
			}

			require.Equal(t, tc.want, isTransientStorageError(resp))
		})
	}
}

func Test_isFile(t *testing.T) {
	dir := t.TempDir()

	file := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0o600))

	// A path below a file is not a directory, so stat fails with something other
	// than fs.ErrNotExist, which must not be dereferenced.
	require.False(t, isFile(filepath.Join(file, "below-a-file")))

	require.True(t, isFile(file))
	require.False(t, isFile(dir))
	require.False(t, isFile(filepath.Join(dir, "missing")))
}

func Test_waitForTCPPort_contextDone(t *testing.T) {
	// The interval is long and the deadline is short, so the context is only
	// observed, if the wait between two attempts observes it as well.
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()

	// The address is deliberately one, which is not listening, so only the
	// context can end the wait.
	err := waitForTCPPort(ctx, t, "127.0.0.1:1", 30*time.Second)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 5*time.Second)
}

func Test_errNotAnIncusOSImage(t *testing.T) {
	// image returns the content of a file of the given size, which carries the
	// GPT header at the given offset. A negative offset carries none.
	image := func(size int, magicOffset int) []byte {
		content := make([]byte, size)
		if magicOffset >= 0 {
			copy(content[magicOffset:], gptHeaderMagic)
		}

		return content
	}

	tests := []struct {
		name    string
		content []byte

		wantErr string
	}{
		{
			name:    "image with a 4096 byte sector size",
			content: image(incusOSImageMinSize, 2048),
		},
		{
			name:    "image with a 512 byte sector size",
			content: image(incusOSImageMinSize, 512),
		},
		{
			name:    "missing file",
			content: nil,

			wantErr: "Failed to stat",
		},
		{
			name:    "empty file",
			content: []byte{},

			wantErr: "which is below the",
		},
		{
			name:    "API index of the customizer instead of an image",
			content: []byte(`{"type":"sync","status":"Success","status_code":200,"metadata":{}}`),

			wantErr: "which is below the",
		},
		{
			name:    "large file without a GPT header",
			content: image(incusOSImageMinSize, -1),

			wantErr: "carries no GPT header",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), operationsCenterISOName)
			if tc.content != nil {
				require.NoError(t, os.WriteFile(path, tc.content, 0o600))
			}

			err := errNotAnIncusOSImage(path)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func Test_tailMsg(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int

		want string
	}{
		{
			name: "empty",
			in:   "",
			n:    3,

			want: "",
		},
		{
			name: "fewer lines than n",
			in:   "a\nb\n",
			n:    3,

			want: "a\nb",
		},
		{
			name: "more lines than n",
			in:   "a\nb\nc\nd\n",
			n:    2,

			want: "[truncated, showing the last 2 of 4 lines]\nc\nd",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tailMsg(tc.in, tc.n))
		})
	}
}

func Test_sanitizeConsoleLog(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		maxBytes int

		want string
	}{
		{
			name:     "plain text is kept as is",
			in:       "Started incus-osd.service\n",
			maxBytes: 1024,

			want: "Started incus-osd.service\n",
		},
		{
			name:     "escape sequences are stripped",
			in:       "\x1b[2J\x1b[001;001H\x1b[?25lBdsDxe: loading\x1b(B\x1b[m\n",
			maxBytes: 1024,

			want: "BdsDxe: loading\n",
		},
		{
			name:     "repeated escape characters are stripped",
			in:       "\x1b\x1b[>4;2m\x1b\x1b[1;1H\x1b\x1b(B\x1b\x1b[m !! IncusOS critical startup error !! ",
			maxBytes: 1024,

			want: " !! IncusOS critical startup error !! ",
		},
		{
			name:     "oversized input is truncated to the tail",
			in:       "0123456789",
			maxBytes: 4,

			want: "[truncated, showing the last 4 of 10 bytes]\n6789",
		},
		{
			name:     "a multi byte rune is not cut in half",
			in:       "ab╔╔",
			maxBytes: 4,

			want: "[truncated, showing the last 3 of 8 bytes]\n╔",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, sanitizeConsoleLog(tc.in, tc.maxBytes))
		})
	}
}

func Test_incusOSStartupError(t *testing.T) {
	tests := []struct {
		name    string
		console string

		wantFragment string
		wantFound    bool
	}{
		{
			name:    "empty",
			console: "",

			wantFound: false,
		},
		{
			name:    "healthy boot",
			console: "Sep 13 06:06:26 localhost systemd[1]: Started incus-osd.service - IncusOS - management daemon.\nSep 13 06:08:11 localhost incus-osd[685]: INFO System is ready version=202609120242\n",

			wantFound: false,
		},
		{
			name:    "incus-osd exited with a failure",
			console: "Sep 13 06:06:14 localhost incus-osd[676]: Error: unable to configure incus-agent: Failed to run: systemctl restart incus-agent.service: exit status 1\nSep 13 06:06:29 localhost systemd[1]: incus-osd.service: Failed with result 'exit-code'.\n",

			wantFragment: "incus-osd.service: Failed with result",
			wantFound:    true,
		},
		{
			name:    "error screen drawn with escape sequences",
			console: "\x1b\x1b[>4;2m\x1b\x1b[1;1H\x1b\x1b(B\x1b\x1b[m╔══ !! IncusOS critical startup error !! ══╗",

			wantFragment: "IncusOS critical startup error",
			wantFound:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fragment, found := incusOSStartupError(tc.console)

			require.Equal(t, tc.wantFound, found)
			require.Equal(t, tc.wantFragment, fragment)
		})
	}
}
