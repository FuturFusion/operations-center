//go:build !linux

package file_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/file"
)

func TestUsageInformationForPath(t *testing.T) {
	t.Parallel()

	gotUI, err := file.UsageInformationForPath(t.TempDir())

	require.ErrorIs(t, err, errors.ErrUnsupported, "usage information is only supported on Linux")
	require.Equal(t, file.UsageInformation{}, gotUI)
}
