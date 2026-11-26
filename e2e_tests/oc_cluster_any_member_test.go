package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	anyMemberChannelName      = "e2e-any-member"
	anyMemberTokenDescription = "e2e-any-member"
	anyMemberLogLevel         = "WARN"
	anyMemberScriptlet        = `def server_registration(candidate):
  server.set_description("e2e any member")
`
)

// ocClusterWriteThroughAnyMember changes data through one member and reads it
// through the other members.
func ocClusterWriteThroughAnyMember(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	members := mustTrustOperationsCenterMembers(ctx, t, tmpDir, operationsCenterVMNames())

	assertChannelThroughAnyMember(t, members)
	assertTokenThroughAnyMember(t, members)
	assertSettingsThroughAnyMember(t, tmpDir, members)
}

// assertChannelThroughAnyMember adds, changes and removes a channel, each step
// through another member.
func assertChannelThroughAnyMember(t *testing.T, members []operationsCenterMember) {
	t.Helper()

	t.Log("Assert a channel changed through any member")

	t.Cleanup(leftoverCleanup(t, "channel "+anyMemberChannelName, `../bin/operations-center.linux.%s provisioning channel remove %s`, cpuArch, anyMemberChannelName))

	mustRun(t, `%s provisioning channel add %s --description created`, members[1].oc, anyMemberChannelName)

	for _, member := range members {
		mustRun(t, `%s provisioning channel show %s -f json | jq -e '.description == "created"'`, member.oc, anyMemberChannelName)
	}

	mustRun(t, `printf 'description: changed\n' | %s provisioning channel edit %s`, members[0].oc, anyMemberChannelName)

	for _, member := range members {
		mustRun(t, `%s provisioning channel show %s -f json | jq -e '.description == "changed"'`, member.oc, anyMemberChannelName)
	}

	mustRun(t, `%s provisioning channel remove %s`, members[2].oc, anyMemberChannelName)

	for _, member := range members {
		mustRun(t, `%s provisioning channel list -f json | jq -e '[ .[] | select(.name == "%s") ] | length == 0'`, member.oc, anyMemberChannelName)
	}
}

// assertTokenThroughAnyMember adds, changes and removes a token, each step
// through another member.
func assertTokenThroughAnyMember(t *testing.T, members []operationsCenterMember) {
	t.Helper()

	t.Log("Assert a token changed through any member")

	mustRun(t, `%s provisioning token add --description %s --uses 5`, members[1].oc, anyMemberTokenDescription)

	resp := mustRun(t, `%s provisioning token list -f json | jq -r -e '[ .[] | select(.description == "%s") ] | select(length == 1) | first | .uuid'`, members[2].oc, anyMemberTokenDescription)
	token := resp.OutputTrimmed()

	t.Cleanup(leftoverCleanup(t, "token "+token, `../bin/operations-center.linux.%s provisioning token remove %s`, cpuArch, token))

	for _, member := range members {
		mustRun(t, `%s provisioning token show %s -f json | jq -e '.uses_remaining == 5'`, member.oc, token)
	}

	mustRun(t, `%[1]s provisioning token show %[2]s -f json | jq '{uses_remaining: 3, expire_at, description, channel}' | %[1]s provisioning token edit %[2]s`, members[0].oc, token)

	for _, member := range members {
		mustRun(t, `%s provisioning token show %s -f json | jq -e '.uses_remaining == 3'`, member.oc, token)
	}

	mustRun(t, `%s provisioning token remove %s`, members[2].oc, token)

	for _, member := range members {
		mustRun(t, `%s provisioning token list -f json | jq -e '[ .[] | select(.uuid == "%s") ] | length == 0'`, member.oc, token)
	}
}

// assertSettingsThroughAnyMember changes the settings through one member. The
// registration scriptlet is shared by all the members. The log level is local
// to the member.
func assertSettingsThroughAnyMember(t *testing.T, tmpDir string, members []operationsCenterMember) {
	t.Helper()

	t.Log("Assert the shared and the local settings changed through one member")

	changed := members[1]

	original := map[string]map[string]any{}
	for _, member := range members {
		original[member.name] = mustOperationsCenterMemberSettings(t, member)
	}

	etagBefore := mustSameSettingsETag(t, members)

	// Restoring the settings through the changed member restores its local
	// part and the shared part.
	t.Cleanup(settingsRestoreCleanup(t, tmpDir, changed, original[changed.name]))

	settings := mustOperationsCenterMemberSettings(t, changed)
	settings["log_level"] = anyMemberLogLevel
	settings["server_registration_scriptlet"] = anyMemberScriptlet

	filename := mustWriteSettings(t, tmpDir, changed, settings)
	mustRun(t, `%s system settings edit < %s`, changed.oc, filename)

	for _, member := range members {
		settings := mustOperationsCenterMemberSettings(t, member)
		require.Equal(t, anyMemberScriptlet, settings["server_registration_scriptlet"], "expect %s to use the shared registration scriptlet", member.name)

		wantLogLevel := original[member.name]["log_level"]
		if member.name == changed.name {
			wantLogLevel = anyMemberLogLevel
		}

		require.Equal(t, wantLogLevel, settings["log_level"], "expect %s to keep its own log level", member.name)
	}

	etagAfter := mustSameSettingsETag(t, members)
	require.NotEqual(t, etagBefore, etagAfter, "expect the ETag to change with the shared settings")
}

// mustOperationsCenterMemberSettings returns the settings of member.
func mustOperationsCenterMemberSettings(t *testing.T, member operationsCenterMember) map[string]any {
	t.Helper()

	resp := mustRun(t, `%s system settings show -f json`, member.oc)

	var settings map[string]any
	err := json.Unmarshal(resp.output.Bytes(), &settings)
	require.NoError(t, err, "Failed to decode the settings of %s", member.name)

	return settings
}

// mustWriteSettings writes settings to a file in tmpDir and returns its name.
// "system settings edit" reads JSON, since JSON is valid YAML.
func mustWriteSettings(t *testing.T, tmpDir string, member operationsCenterMember, settings map[string]any) string {
	t.Helper()

	content, err := json.Marshal(settings)
	require.NoError(t, err)

	filename := filepath.Join(tmpDir, fmt.Sprintf("system_settings_%s.json", member.name))
	err = os.WriteFile(filename, content, 0o600)
	require.NoError(t, err)

	return filename
}

// mustSameSettingsETag asserts that all the members return the same ETag for
// the settings and returns it. The ETag only covers the shared part.
func mustSameSettingsETag(t *testing.T, members []operationsCenterMember) string {
	t.Helper()

	clientAuth := newOperationsCenterAPI(t).clientAuth

	var etag string
	for _, member := range members {
		resp := mustRun(t, `curl -sSfk %s -o /dev/null -D - https://%s/1.0/system/settings | grep -i '^etag:'`, clientAuth, member.address)
		_, value, _ := strings.Cut(resp.OutputTrimmed(), ":")
		value = strings.TrimSpace(value)
		require.NotEmpty(t, value, "expect %s to return an ETag for the settings", member.name)

		if etag == "" {
			etag = value
		}

		require.Equal(t, etag, value, "expect %s to return the same ETag for the settings", member.name)
	}

	return etag
}

// settingsRestoreCleanup restores the settings of member.
func settingsRestoreCleanup(t *testing.T, tmpDir string, member operationsCenterMember, settings map[string]any) func() {
	t.Helper()

	filename := mustWriteSettings(t, tmpDir, member, settings)

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(time.Minute))
		defer cancel()

		resp := runWithContext(ctx, t, `%s system settings edit < %s`, member.oc, filename)
		if !resp.Success() {
			t.Errorf("Failed to restore the settings of %s: %s", member.name, resp.Error())
		}
	}
}

// leftoverCleanup runs command with args, which removes the leftover what. The
// test removes it itself, so the command usually fails.
func leftoverCleanup(t *testing.T, what string, command string, args ...any) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		resp := runWithContext(ctx, t, command, args...)
		if resp.Success() {
			t.Logf("Removed the leftover %s during cleanup", what)
		}
	}
}
