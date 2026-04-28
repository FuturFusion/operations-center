package provisioning

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
	"github.com/FuturFusion/operations-center/internal/warning"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestLogWarningService(t *testing.T) {
	logBuf := &bytes.Buffer{}
	err := logger.InitLogger(logBuf, "", false, true, true)
	require.NoError(t, err)

	warn := LogWarningService{}

	warn.Emit(t.Context(), warning.NewWarning(
		api.WarningTypeUnreachable,
		api.WarningScope{
			Scope:      "test",
			EntityType: "test",
			Entity:     "1",
		},
		"boom!",
	))
	warn.RemoveStale(t.Context(), api.WarningScope{}, nil)

	log.Match(`boom!.*type="Server unreachable" scope=test entity_type=test entity=1`)(t, logBuf)
}
