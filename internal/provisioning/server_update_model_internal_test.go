package provisioning

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_serverUpdateSteps(t *testing.T) {
	for step, definition := range serverUpdateSteps {
		t.Run(string(step), func(t *testing.T) {
			require.NotEmpty(t, definition.details, "a step records a status detail, while it waits for its outcome")
			require.NotZero(t, definition.timeout, "a step without a timeout is never in flight")
			require.NotZero(t, definition.retries, "a step without attempts fails on its first trigger")
		})
	}
}
