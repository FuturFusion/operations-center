package provisioning

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerDeployRecordBMCFlag(t *testing.T) {
	t.Parallel()

	// Setup
	deploy := cmdServerDeploy{}

	// Run test
	flag := deploy.Command().Flags().Lookup("record-bmc")

	// Assert
	require.NotNil(t, flag, "the flag exists")
	require.True(t, flag.Hidden, "the flag is a development feature and not listed in the help")
	require.Contains(t, flag.Usage, "for development")
}
