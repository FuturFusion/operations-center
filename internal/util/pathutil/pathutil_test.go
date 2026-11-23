package pathutil_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/pathutil"
)

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
			got := pathutil.IsPathElement(tc.pathname)

			// Assert
			require.Equal(t, tc.want, got)
		})
	}
}
