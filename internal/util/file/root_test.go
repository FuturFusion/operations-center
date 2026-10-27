package file_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
)

func TestMkdirAllOpenRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		dir   string

		assertErr  require.ErrorAssertionFunc
		assertRoot func(t *testing.T, root *os.Root, dir string)
		wantFiles  []string
	}{
		{
			name: "success - missing directories are created",
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},
			dir: "a/b",

			assertErr: require.NoError,
			assertRoot: func(t *testing.T, root *os.Root, dir string) {
				t.Helper()
				require.Equal(t, dir, root.Name())
			},
			wantFiles: []string{"a", "a/b"},
		},
		{
			name: "success - directory exists",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				err := os.Mkdir(filepath.Join(dir, "a"), 0o700)
				require.NoError(t, err)
			},
			dir: "a",

			assertErr: require.NoError,
			assertRoot: func(t *testing.T, root *os.Root, dir string) {
				t.Helper()
				require.Equal(t, dir, root.Name())
			},
			wantFiles: []string{"a"},
		},
		{
			name: "error - path component is a file",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(dir, "a"), nil, 0o600)
				require.NoError(t, err)
			},
			dir: "a/b",

			assertErr: require.Error,
			assertRoot: func(t *testing.T, root *os.Root, dir string) {
				t.Helper()
				require.Nil(t, root)
			},
			wantFiles: []string{"a"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			tc.setup(t, tmpDir)
			dir := filepath.Join(tmpDir, tc.dir)

			// Run test
			root, err := file.MkdirAllOpenRoot(dir, 0o700)

			// Assert
			tc.assertErr(t, err)
			tc.assertRoot(t, root, dir)
			require.Equal(t, tc.wantFiles, listFiles(t, tmpDir))
		})
	}
}

func TestMkdirAllOpenSubRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string)
		subRoot string

		assertErr  require.ErrorAssertionFunc
		assertRoot func(t *testing.T, root *os.Root)
		wantFiles  []string
	}{
		{
			name: "success - missing directories are created",
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},
			subRoot: "a/b",

			assertErr: require.NoError,
			assertRoot: func(t *testing.T, root *os.Root) {
				t.Helper()
				require.NotNil(t, root)
			},
			wantFiles: []string{"parent", "parent/a", "parent/a/b"},
		},
		{
			name: "error - path escapes parent root",
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},
			subRoot: "../escape",

			assertErr: require.Error,
			assertRoot: func(t *testing.T, root *os.Root) {
				t.Helper()
				require.Nil(t, root)
			},
			wantFiles: []string{"parent"},
		},
		{
			name: "error - path component is a file",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(dir, "a"), nil, 0o600)
				require.NoError(t, err)
			},
			subRoot: "a/b",

			assertErr: require.Error,
			assertRoot: func(t *testing.T, root *os.Root) {
				t.Helper()
				require.Nil(t, root)
			},
			wantFiles: []string{"parent", "parent/a"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			parent := openTestRoot(t, tmpDir, "parent")
			tc.setup(t, parent.Name())

			// Run test
			root, err := file.MkdirAllOpenSubRoot(parent, tc.subRoot, 0o700)

			// Assert
			tc.assertErr(t, err)
			tc.assertRoot(t, root)
			require.Equal(t, tc.wantFiles, listFiles(t, tmpDir), "nothing must be created outside of the parent root")
		})
	}
}

func TestPathExistsIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(t *testing.T, dir string)
		pathname string

		want bool
	}{
		{
			name: "file exists",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(dir, "file"), nil, 0o600)
				require.NoError(t, err)
			},
			pathname: "file",

			want: true,
		},
		{
			name: "dangling symlink exists",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				err := os.Symlink("missing", filepath.Join(dir, "link"))
				require.NoError(t, err)
			},
			pathname: "link",

			want: true,
		},
		{
			name: "file does not exist",
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},
			pathname: "missing",

			want: false,
		},
		{
			name: "path escaping the root counts as existing",
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},
			pathname: "../missing",

			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			root := openTestRoot(t, t.TempDir(), "root")
			tc.setup(t, root.Name())

			// Run test
			got := file.PathExistsIn(root, tc.pathname)

			// Assert
			require.Equal(t, tc.want, got, "only a missing path is reported as not existing, like PathExists does")
		})
	}
}

func TestCreateTempIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string)
		dir     string
		pattern string

		assertErr     require.ErrorAssertionFunc
		wantNameRegex string
		assertFile    func(t *testing.T, root *os.Root, f *os.File, name string)
	}{
		{
			name: "success - pattern with wildcard",
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},
			dir:     ".",
			pattern: "file.*.partial",

			assertErr:     require.NoError,
			wantNameRegex: `^file\.[0-9]+\.partial$`,
			assertFile:    assertTempFile,
		},
		{
			name: "success - pattern without wildcard in sub directory",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				err := os.Mkdir(filepath.Join(dir, "sub"), 0o700)
				require.NoError(t, err)
			},
			dir:     "sub",
			pattern: "file-",

			assertErr:     require.NoError,
			wantNameRegex: `^sub/file-[0-9]+$`,
			assertFile:    assertTempFile,
		},
		{
			name: "error - pattern contains path separator",
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},
			dir:     ".",
			pattern: "../file.*",

			assertErr:     require.Error,
			wantNameRegex: `^$`,
			assertFile:    assertNoTempFile,
		},
		{
			name: "error - directory escapes the root",
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},
			dir:     "..",
			pattern: "file.*",

			assertErr:     require.Error,
			wantNameRegex: `^$`,
			assertFile:    assertNoTempFile,
		},
		{
			name: "error - directory does not exist",
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},
			dir:     "missing",
			pattern: "file.*",

			assertErr:     require.Error,
			wantNameRegex: `^$`,
			assertFile:    assertNoTempFile,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			root := openTestRoot(t, tmpDir, "root")
			tc.setup(t, root.Name())

			// Run test
			f, name, err := file.CreateTempIn(root, tc.dir, tc.pattern)

			// Assert
			tc.assertErr(t, err)
			require.Regexp(t, tc.wantNameRegex, name)
			tc.assertFile(t, root, f, name)

			entries, err := os.ReadDir(tmpDir)
			require.NoError(t, err)
			require.Len(t, entries, 1, "nothing must be created outside of the root")
		})
	}
}

func TestMkdirTempIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dir     string
		pattern string

		assertErr     require.ErrorAssertionFunc
		wantNameRegex string
	}{
		{
			name:    "success",
			dir:     ".",
			pattern: "tmp-*",

			assertErr:     require.NoError,
			wantNameRegex: `^tmp-[0-9]+$`,
		},
		{
			name:    "error - pattern contains path separator",
			dir:     ".",
			pattern: "../tmp-*",

			assertErr:     require.Error,
			wantNameRegex: `^$`,
		},
		{
			name:    "error - directory escapes the root",
			dir:     "..",
			pattern: "tmp-*",

			assertErr:     require.Error,
			wantNameRegex: `^$`,
		},
		{
			name:    "error - directory does not exist",
			dir:     "missing",
			pattern: "tmp-*",

			assertErr:     require.Error,
			wantNameRegex: `^$`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			root := openTestRoot(t, tmpDir, "root")

			// Run test
			name, err := file.MkdirTempIn(root, tc.dir, tc.pattern)

			// Assert
			tc.assertErr(t, err)
			require.Regexp(t, tc.wantNameRegex, name)

			entries, err := os.ReadDir(tmpDir)
			require.NoError(t, err)
			require.Len(t, entries, 1, "nothing must be created outside of the root")
		})
	}
}

func TestWriteFileAtomicIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(t *testing.T, dir string)
		filename string

		assertErr require.ErrorAssertionFunc
		wantFiles map[string]string
	}{
		{
			name: "success - create file",
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},
			filename: "target",

			assertErr: require.NoError,
			wantFiles: map[string]string{
				"root/target": "contents",
			},
		},
		{
			name: "success - replace file in sub directory",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				err := os.Mkdir(filepath.Join(dir, "sub"), 0o700)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(dir, "sub", "target"), []byte("old"), 0o644)
				require.NoError(t, err)
			},
			filename: "sub/target",

			assertErr: require.NoError,
			wantFiles: map[string]string{
				"root/sub/target": "contents",
			},
		},
		{
			name: "error - file escapes the root",
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},
			filename: "../target",

			assertErr: require.Error,
			wantFiles: map[string]string{},
		},
		{
			name: "error - directory does not exist",
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},
			filename: "missing/target",

			assertErr: require.Error,
			wantFiles: map[string]string{},
		},
		{
			name: "error - target is a non-empty directory",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				err := os.Mkdir(filepath.Join(dir, "target"), 0o700)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(dir, "target", "inner"), []byte("old"), 0o600)
				require.NoError(t, err)
			},
			filename: "target",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to move temporary file into place", a...)
			},
			wantFiles: map[string]string{
				"root/target/inner": "old",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			root := openTestRoot(t, tmpDir, "root")
			tc.setup(t, root.Name())

			// Run test
			err := file.WriteFileAtomicIn(root, tc.filename, []byte("contents"), 0o600)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantFiles, readFiles(t, tmpDir), "the temporary file must be removed and nothing must be written outside of the root")
		})
	}
}

func TestWriteFileAtomicIn_permissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		perm os.FileMode
	}{
		{
			name: "owner only",
			perm: 0o600,
		},
		{
			name: "group readable",
			perm: 0o640,
		},
		{
			name: "world readable",
			perm: 0o644,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			root := openTestRoot(t, t.TempDir(), "root")

			// Run test
			err := file.WriteFileAtomicIn(root, "target", []byte("contents"), tc.perm)

			// Assert
			require.NoError(t, err)
			info, err := root.Stat("target")
			require.NoError(t, err)
			require.Equal(t, tc.perm, info.Mode().Perm(), "the file must have the requested permissions")
		})
	}
}

func openTestRoot(t *testing.T, parentDir string, name string) *os.Root {
	t.Helper()

	dir := filepath.Join(parentDir, name)
	err := os.Mkdir(dir, 0o700)
	require.NoError(t, err)

	return fsroot.Open(t, dir)
}

func assertTempFile(t *testing.T, root *os.Root, f *os.File, name string) {
	t.Helper()

	require.NoError(t, f.Close())

	fileInfo, err := root.Stat(name)
	require.NoError(t, err)
	require.True(t, fileInfo.Mode().IsRegular())
	require.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm(), "temporary file is only accessible by the owner")
}

func assertNoTempFile(t *testing.T, root *os.Root, f *os.File, name string) {
	t.Helper()

	require.Nil(t, f)
}

// listFiles returns the relative names of all files and directories in dir.
func listFiles(t *testing.T, dir string) []string {
	t.Helper()

	files := []string{}
	err := filepath.WalkDir(dir, func(name string, d fs.DirEntry, err error) error {
		require.NoError(t, err)

		rel, err := filepath.Rel(dir, name)
		require.NoError(t, err)

		if rel != "." {
			files = append(files, rel)
		}

		return nil
	})
	require.NoError(t, err)

	return files
}

// readFiles returns the contents of all regular files in dir by relative name.
func readFiles(t *testing.T, dir string) map[string]string {
	t.Helper()

	files := map[string]string{}
	err := filepath.WalkDir(dir, func(name string, d fs.DirEntry, err error) error {
		require.NoError(t, err)

		if !d.Type().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(dir, name)
		require.NoError(t, err)

		contents, err := os.ReadFile(name)
		require.NoError(t, err)

		files[rel] = string(contents)

		return nil
	})
	require.NoError(t, err)

	return files
}

func TestIsPathElement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pathname string

		want bool
	}{
		{name: "file name", pathname: "file.txt", want: true},
		{name: "name with dots", pathname: "..file..", want: true},
		{name: "empty", pathname: "", want: false},
		{name: "current directory", pathname: ".", want: false},
		{name: "parent directory", pathname: "..", want: false},
		{name: "relative path", pathname: "dir/file.txt", want: false},
		{name: "absolute path", pathname: "/file.txt", want: false},
		{name: "backslash", pathname: `..\file.txt`, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			got := file.IsPathElement(tc.pathname)

			// Assert
			require.Equal(t, tc.want, got)
		})
	}
}
