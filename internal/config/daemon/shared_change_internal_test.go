package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSharedChange(t *testing.T) {
	stored := SharedConfig{}
	stored.Settings.ServerRegistrationScriptlet = "stored by another member"
	stored.Security.TrustedTLSClientCertFingerprints = []string{"previous"}

	// The member changing the security section missed the settings change.
	stale := SharedConfig{}
	stale.Security.TrustedTLSClientCertFingerprints = []string{"new"}

	change := SharedChange{section: sectionSecurity, cfg: stale}

	merged := change.Apply(stored)
	require.Equal(t, "stored by another member", merged.Settings.ServerRegistrationScriptlet)
	require.Equal(t, []string{"new"}, merged.Security.TrustedTLSClientCertFingerprints)
	require.True(t, change.Applied(merged))

	// A later change of another section keeps the change applied.
	later := merged
	later.Settings.ServerRegistrationScriptlet = "later"
	require.True(t, change.Applied(later))

	reverted := change.Revert(later, stored)
	require.Equal(t, "later", reverted.Settings.ServerRegistrationScriptlet)
	require.Equal(t, []string{"previous"}, reverted.Security.TrustedTLSClientCertFingerprints)

	// A later change of the same section is not this change anymore.
	replaced := merged
	replaced.Security.TrustedTLSClientCertFingerprints = []string{"other"}
	require.False(t, change.Applied(replaced))

	// A change of all sections replaces the stored config.
	require.Equal(t, stale, SharedChange{section: sectionChanged, cfg: stale}.Apply(stored))
}
