package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const (
	provisioningChannelName = "e2e-provisioning"

	provisioningTokenDescription        = "e2e-provisioning-token"
	provisioningTokenEditedDescription  = "e2e-provisioning-token-edited"
	provisioningExpiredTokenDescription = "e2e-provisioning-token-expired"
	provisioningSeedTokenDescription    = "e2e-provisioning-token-seed"
)

// incusOSArchitecture returns the name, IncusOS uses for the CPU architecture
// under test.
func incusOSArchitecture(t *testing.T) string {
	t.Helper()

	switch cpuArch {
	case "amd64":
		return "x86_64"

	case "arm64":
		return "aarch64"
	}

	t.Fatalf("CPU architecture %q is not supported by IncusOS", cpuArch)

	return ""
}

func provisioningUpdatesChannelsAndTokens(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	ocAPI := newOperationsCenterAPI(t)

	assertProvisioningUpdates(t, ocAPI)
	assertProvisioningChannels(t, tmpDir, ocAPI)
	assertSystemUpdatesDefaultChannels(t, tmpDir)
	assertProvisioningTokens(ctx, t, tmpDir, ocAPI)
	assertProvisioningTokenSeedImages(t, tmpDir, ocAPI)
}

func assertProvisioningUpdates(t *testing.T, ocAPI operationsCenterAPI) {
	t.Helper()

	t.Log("Assert provisioning updates")

	// Setup
	// Only upstream updates are used, since manually uploaded updates of other
	// tests might lack the upstream channel.
	updateUUID := mustRun(t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -r -e '[ .[] | select(.update_status == "ready" and (.channels | index("stable")) and (.upstream_channels | index("stable"))) ] | sort_by(.version) | last | .uuid'`, cpuArch).OutputTrimmed()
	version := mustRun(t, `../bin/operations-center.linux.%s provisioning update show %s -f json | jq -r -e '.version'`, cpuArch, updateUUID).OutputTrimmed()
	origin := mustRun(t, `../bin/operations-center.linux.%s provisioning update show %s -f json | jq -r -e '.origin'`, cpuArch, updateUUID).OutputTrimmed()

	// The changelog compares with the next older update of the channel, no matter its status.
	priorVersion := mustRun(t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -r '[ .[] | select(.channels | index("stable")) | .version ] | unique | reverse | .[index("%s") + 1] // ""'`, cpuArch, version).OutputTrimmed()

	// Show
	resp := mustRun(t, `../bin/operations-center.linux.%s provisioning update show %s`, cpuArch, updateUUID)
	require.Contains(t, resp.Output(), "UUID: "+updateUUID, "expect update show to name the update")
	require.Contains(t, resp.Output(), "Files:", "expect update show to list the files of the update")

	// Files
	mustRun(t, `../bin/operations-center.linux.%s provisioning update file list %s -f json | jq -r -e 'length > 0 and all(.sha256 | length == 64)'`, cpuArch, updateUUID)

	// The smallest file keeps the download short.
	filename := mustRun(t, `../bin/operations-center.linux.%s provisioning update file list %s -f json | jq -r -e 'sort_by(.size) | first | .filename'`, cpuArch, updateUUID).OutputTrimmed()
	sha256 := mustRun(t, `../bin/operations-center.linux.%s provisioning update file list %s -f json | jq -r -e '.[] | select(.filename == "%s") | .sha256'`, cpuArch, updateUUID, filename).OutputTrimmed()

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning update file show %s %s`, cpuArch, updateUUID, filename)
	require.Contains(t, resp.Output(), "SHA256: "+sha256, "expect update file show to report the SHA-256 sum of file list")

	target := filepath.Join(t.TempDir(), "update-file")
	mustRun(t, `../bin/operations-center.linux.%s provisioning update file get %s %s %s`, cpuArch, updateUUID, filename, target)
	require.Equal(t, sha256, mustSHA256(t, target), "expect the file fetched by update file get to match its SHA-256 sum")

	// The GET routes of the updates are served without authentication.
	resp = mustRun(t, `curl -sSk --fail-with-body "%s/provisioning/updates/%s/files/%s" | sha256sum | cut -d ' ' -f 1`, ocAPI.baseURL, updateUUID, filename)
	require.Equal(t, sha256, resp.OutputTrimmed(), "expect the file served without authentication to match its SHA-256 sum")

	allUpdates := mustRun(t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -r '[ .[].uuid ] | sort | join(",")'`, cpuArch).OutputTrimmed()
	resp = mustRun(t, `curl -sSk --fail-with-body "%s/provisioning/updates?recursion=1" | jq -r '[ .metadata[].uuid ] | sort | join(",")'`, ocAPI.baseURL)
	require.Equal(t, allUpdates, resp.OutputTrimmed(), "expect the updates listed without authentication to match the updates of the CLI")

	// The other routes require authentication.
	mustCurl(t, http.StatusUnauthorized, `-X POST "%s/provisioning/updates/:refresh"`, ocAPI.baseURL)

	// Changelog
	resp = mustRun(t, `curl -sSk --fail-with-body "%s/provisioning/updates/%s/changelog?channel=stable&upstream=false&architecture=%s" | jq -r -e '.metadata | [ .current_version, .prior_version, .channel ] | join(",")'`, ocAPI.baseURL, updateUUID, incusOSArchitecture(t))
	require.Equal(t, strings.Join([]string{version, priorVersion, "stable"}, ","), resp.OutputTrimmed(), "expect the changelog to compare with the next older update of the channel")

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning update changelog %s --channel stable`, cpuArch, updateUUID)
	require.Contains(t, resp.Output(), version, "expect update changelog to name the version of the update")

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning update changelog %s --channel stable --upstream`, cpuArch, updateUUID)
	require.Contains(t, resp.Output(), version, "expect update changelog of the upstream channel to name the version of the update")

	// List filters
	filters := []struct {
		flag   string
		jqExpr string
	}{
		{flag: "--channel stable", jqExpr: `.channels | index("stable")`},
		{flag: "--status ready", jqExpr: `.update_status == "ready"`},
		{flag: fmt.Sprintf("--origin %q", origin), jqExpr: fmt.Sprintf(`.origin == %q`, origin)},
		{flag: "--upstream-channel stable", jqExpr: `.upstream_channels | index("stable")`},
	}

	for _, filter := range filters {
		want := mustRun(t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -r '[ .[] | select(%s) | .uuid ] | sort | join(",")'`, cpuArch, filter.jqExpr).OutputTrimmed()
		got := mustRun(t, `../bin/operations-center.linux.%s provisioning update list %s -f json | jq -r '[ .[].uuid ] | sort | join(",")'`, cpuArch, filter.flag).OutputTrimmed()

		require.NotEmpty(t, want, "expect at least one update to match %s", filter.flag)
		require.Equal(t, want, got, "expect update list %s to return the matching updates only", filter.flag)
	}
}

func assertProvisioningChannels(t *testing.T, tmpDir string, ocAPI operationsCenterAPI) {
	t.Helper()

	t.Log("Assert provisioning channels")

	// Setup
	resp := mustRun(t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -r -e '[ .[] | select(.update_status == "ready" and (.channels | index("stable")) and (.upstream_channels | index("stable"))) ] | sort_by(.version) | reverse | .[0:2] | .[].uuid'`, cpuArch)
	updateUUIDs := strings.Fields(resp.Output())
	require.Len(t, updateUUIDs, 2, "expect at least 2 ready updates in the stable channel")

	originalChannels := make([]string, 0, len(updateUUIDs))
	versions := make([]string, 0, len(updateUUIDs))
	for _, updateUUID := range updateUUIDs {
		originalChannels = append(originalChannels, mustRun(t, `../bin/operations-center.linux.%s provisioning update show %s -f json | jq -r -e '.channels | join(",")'`, cpuArch, updateUUID).OutputTrimmed())
		versions = append(versions, mustRun(t, `../bin/operations-center.linux.%s provisioning update show %s -f json | jq -r -e '.version'`, cpuArch, updateUUID).OutputTrimmed())
	}

	t.Cleanup(provisioningChannelCleanup(t, provisioningChannelName, updateUUIDs, originalChannels))

	// Add
	mustRun(t, `../bin/operations-center.linux.%s provisioning channel add %s --description "E2E channel"`, cpuArch, provisioningChannelName)
	mustRun(t, `../bin/operations-center.linux.%s provisioning channel list -f json | jq -r -e 'any(.name == "%s")'`, cpuArch, provisioningChannelName)

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning channel show %s -f json | jq -r -e '.description'`, cpuArch, provisioningChannelName)
	require.Equal(t, "E2E channel", resp.OutputTrimmed(), "expect channel show to return the description given to channel add")

	// Edit
	channelPutFile := filepath.Join(tmpDir, "channel_put.yaml")
	err := os.WriteFile(channelPutFile, []byte("description: E2E channel edited\n"), 0o600)
	require.NoError(t, err)

	mustRun(t, `../bin/operations-center.linux.%s provisioning channel edit %s < %s`, cpuArch, provisioningChannelName, channelPutFile)

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning channel show %s -f json | jq -r -e '.description'`, cpuArch, provisioningChannelName)
	require.Equal(t, "E2E channel edited", resp.OutputTrimmed(), "expect channel edit to update the description")

	// Changelog
	resp = run(t, `../bin/operations-center.linux.%s provisioning channel changelog %s`, cpuArch, provisioningChannelName)
	require.False(t, resp.Success(), "expect channel changelog to fail for a channel without updates")
	require.Contains(t, resp.Output(), "does not contain any updates", "expect channel changelog to name the missing updates")

	for i, updateUUID := range updateUUIDs {
		mustRun(t, `../bin/operations-center.linux.%s provisioning update assign-channels %s --channel %s,%s`, cpuArch, updateUUID, originalChannels[i], provisioningChannelName)
	}

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning update list --channel %s -f json | jq -r '[ .[].uuid ] | sort | join(",")'`, cpuArch, provisioningChannelName)
	require.Equal(t, strings.Join(slices.Sorted(slices.Values(updateUUIDs)), ","), resp.OutputTrimmed(), "expect update list --channel to return the updates assigned to the channel")

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning channel show %s`, cpuArch, provisioningChannelName)
	require.Equal(t, len(updateUUIDs), strings.Count(resp.Output(), "\n- "), "expect channel show to list the assigned updates only")
	for _, updateUUID := range updateUUIDs {
		require.Contains(t, resp.Output(), "- "+updateUUID, "expect channel show to list the assigned update %s", updateUUID)
	}

	resp = mustRun(t, `curl -sSk --fail-with-body %s "%s/provisioning/channels/%s/changelog?architecture=%s" | jq -r -e '.metadata | map([ .current_version, .prior_version ] | join(",")) | join(";")'`, ocAPI.clientAuth, ocAPI.baseURL, provisioningChannelName, incusOSArchitecture(t))
	require.Equal(t, strings.Join(versions, ","), resp.OutputTrimmed(), "expect the changelog of a channel with 2 updates to compare the newer with the older update")

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning channel changelog %s`, cpuArch, provisioningChannelName)
	require.Contains(t, resp.Output(), versions[0], "expect channel changelog to name the version of the newest update")

	// Remove
	resp = run(t, `../bin/operations-center.linux.%s provisioning channel remove %s`, cpuArch, provisioningChannelName)
	require.False(t, resp.Success(), "expect channel remove to be refused while updates are assigned")
	require.Contains(t, resp.Output(), "is assigned to 2 updates", "expect channel remove to name the assigned updates")

	for i, updateUUID := range updateUUIDs {
		mustRun(t, `../bin/operations-center.linux.%s provisioning update assign-channels %s --channel %s`, cpuArch, updateUUID, originalChannels[i])
	}

	mustRun(t, `../bin/operations-center.linux.%s provisioning channel remove %s`, cpuArch, provisioningChannelName)
	mustRun(t, `../bin/operations-center.linux.%s provisioning channel list -f json | jq -r -e 'all(.name != "%s")'`, cpuArch, provisioningChannelName)
}

// provisioningChannelCleanup restores the channels of the given updates and
// removes the channel.
func provisioningChannelCleanup(t *testing.T, channelName string, updateUUIDs []string, originalChannels []string) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		for i, updateUUID := range updateUUIDs {
			resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning update assign-channels %s --channel %s`, cpuArch, updateUUID, originalChannels[i])
			if !resp.Success() {
				t.Error(resp.Error())
			}
		}

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%[1]s provisioning channel list -f json | jq -r -e 'all(.name != "%[2]s")' || ../bin/operations-center.linux.%[1]s provisioning channel remove %[2]s`, cpuArch, channelName)
		if !resp.Success() {
			t.Error(resp.Error())
		}
	}
}

func assertSystemUpdatesDefaultChannels(t *testing.T, tmpDir string) {
	t.Helper()

	t.Log("Assert system updates default channels")

	// Setup
	originalUpdatesFile := filepath.Join(tmpDir, "system_updates_original.yaml")
	mustRun(t, `../bin/operations-center.linux.%s system updates show > %s`, cpuArch, originalUpdatesFile)

	t.Cleanup(func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		mustRunWithContext(ctx, t, `../bin/operations-center.linux.%s system updates edit < %s`, cpuArch, originalUpdatesFile)
	})

	for _, key := range []string{"updates_default_channel", "server_default_channel"} {
		// Run test
		resp := run(t, `../bin/operations-center.linux.%[1]s system updates show | sed "s|^%[2]s: .*|%[2]s: e2e-missing-channel|" | ../bin/operations-center.linux.%[1]s system updates edit`, cpuArch, key)

		// Assert
		require.False(t, resp.Success(), "expect system updates edit to refuse a missing channel for %s", key)
		require.Contains(t, resp.Output(), "updates."+key, "expect system updates edit to name the invalid setting %s", key)

		resp = mustRun(t, `../bin/operations-center.linux.%s system updates show`, cpuArch)
		require.NotContains(t, resp.Output(), "e2e-missing-channel", "expect the refused system updates edit to keep the channels")
	}
}

func assertProvisioningTokens(ctx context.Context, t *testing.T, tmpDir string, ocAPI operationsCenterAPI) {
	t.Helper()

	t.Log("Assert provisioning tokens")

	// Setup
	t.Cleanup(provisioningTokensCleanup(t, provisioningTokenDescription, provisioningTokenEditedDescription, provisioningExpiredTokenDescription))

	serverDefaultChannel := mustRun(t, `../bin/operations-center.linux.%s system updates show -f json | jq -r -e '.server_default_channel'`, cpuArch).OutputTrimmed()

	mustRun(t, `../bin/operations-center.linux.%s provisioning token add --description %s --uses 1 --lifetime 1h`, cpuArch, provisioningTokenDescription)
	token := mustRun(t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -r -e '.[] | select(.description == "%s") | .uuid'`, cpuArch, provisioningTokenDescription).OutputTrimmed()

	registerURL := func(token string) string {
		return fmt.Sprintf("%s/provisioning/servers?token=%s", ocAPI.baseURL, token)
	}

	// A registration presents a client certificate, the body is decoded first.
	const registerArgs = `%s -X POST -H "Content-Type: application/json" -d '{}' "%s"`

	// Show
	resp := mustRun(t, `../bin/operations-center.linux.%s provisioning token show %s -f json | jq -r -e '[ .uses_remaining, .description, .channel ] | map(tostring) | join(",")'`, cpuArch, token)
	require.Equal(t, strings.Join([]string{"1", provisioningTokenDescription, serverDefaultChannel}, ","), resp.OutputTrimmed(), "expect token show to return the values given to token add and the default channel for servers")

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning token show %s`, cpuArch, token)
	require.Contains(t, resp.Output(), "UUID: "+token, "expect token show to name the token")

	// Provider config
	address := mustRun(t, `../bin/operations-center.linux.%s system network show -f json | jq -r -e '.address'`, cpuArch).OutputTrimmed()
	resp = mustRun(t, `../bin/operations-center.linux.%s query /provisioning/tokens/%s/provider-config | jq -r -e '.metadata | [ .name, .version, .config.server_token, .config.server_url ] | join(",")'`, cpuArch, token)
	require.Equal(t, strings.Join([]string{"operations-center", "1", token, address}, ","), resp.OutputTrimmed(), "expect the provider config to point to Operations Center with the token")

	// A failed registration does not consume the token.
	body := mustCurl(t, http.StatusBadRequest, registerArgs, ocAPI.clientAuth, registerURL(token))
	require.Contains(t, body, "name can not be empty", "expect the registration of a server without name to be refused")

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning token show %s -f json | jq -r -e '.uses_remaining'`, cpuArch, token)
	require.Equal(t, "1", resp.OutputTrimmed(), "expect a failed registration to keep the uses of the token")

	// Edit
	expireAt := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	tokenPutFile := filepath.Join(tmpDir, "token_put.yaml")
	err := os.WriteFile(tokenPutFile, fmt.Appendf(nil, "uses_remaining: 0\nexpire_at: %q\ndescription: %s\nchannel: %s\n", expireAt.Format(time.RFC3339), provisioningTokenEditedDescription, serverDefaultChannel), 0o600)
	require.NoError(t, err)

	mustRun(t, `../bin/operations-center.linux.%s provisioning token edit %s < %s`, cpuArch, token, tokenPutFile)

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning token show %s -f json | jq -r -e '[ .uses_remaining, .description, .channel ] | map(tostring) | join(",")'`, cpuArch, token)
	require.Equal(t, strings.Join([]string{"0", provisioningTokenEditedDescription, serverDefaultChannel}, ","), resp.OutputTrimmed(), "expect token edit to update the token")

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning token show %s -f json | jq -r -e '.expire_at'`, cpuArch, token)
	gotExpireAt, err := time.Parse(time.RFC3339Nano, resp.OutputTrimmed())
	require.NoError(t, err)
	require.True(t, expireAt.Equal(gotExpireAt), "expect token edit to update the expiry to %s, got %s", expireAt, gotExpireAt)

	// Registration with an exhausted token.
	body = mustCurl(t, http.StatusBadRequest, registerArgs, ocAPI.clientAuth, registerURL(token))
	require.Contains(t, body, "The token has no uses remaining", "expect the registration with an exhausted token to be refused")

	// Registration with an unknown token.
	body = mustCurl(t, http.StatusForbidden, registerArgs, ocAPI.clientAuth, registerURL(uuid.NewString()))
	require.Contains(t, body, "The token is not valid", "expect the registration with an unknown token to be refused")

	// Registration with an expired token. The expiry is checked by Operations
	// Center, so the clocks of the host and the VM may differ.
	mustRun(t, `../bin/operations-center.linux.%s provisioning token add --description %s --uses 1 --lifetime 5s`, cpuArch, provisioningExpiredTokenDescription)
	expiredToken := mustRun(t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -r -e '.[] | select(.description == "%s") | .uuid'`, cpuArch, provisioningExpiredTokenDescription).OutputTrimmed()

	ok, err := waitForSuccessWithTimeout(ctx, t, "token expired", `curl -sSk `+registerArgs+` | grep -q "The token expired at"`, time.Minute, ocAPI.clientAuth, registerURL(expiredToken))
	require.NoError(t, err)
	require.True(t, ok, "expect the registration with an expired token to be refused")

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning token show %s -f json | jq -r -e '.uses_remaining'`, cpuArch, expiredToken)
	require.Equal(t, "1", resp.OutputTrimmed(), "expect the refused registrations to keep the uses of the expired token")
}

// provisioningTokensCleanup removes the tokens with the given descriptions.
func provisioningTokensCleanup(t *testing.T, descriptions ...string) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -r '.[] | select(.description as $d | %s | index($d)) | .uuid'`, cpuArch, asJSON(t, descriptions))
		if !resp.Success() {
			t.Error(resp.Error())
			return
		}

		for token := range strings.FieldsSeq(resp.Output()) {
			resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning token remove %s`, cpuArch, token)
			if !resp.Success() {
				t.Error(resp.Error())
			}
		}
	}
}

func assertProvisioningTokenSeedImages(t *testing.T, tmpDir string, ocAPI operationsCenterAPI) {
	t.Helper()

	t.Log("Assert provisioning token seed images")

	// Setup
	t.Cleanup(provisioningTokensCleanup(t, provisioningSeedTokenDescription))

	mustRun(t, `../bin/operations-center.linux.%s provisioning token add --description %s --uses 1 --lifetime 1h`, cpuArch, provisioningSeedTokenDescription)
	token := mustRun(t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -r -e '.[] | select(.description == "%s") | .uuid'`, cpuArch, provisioningSeedTokenDescription).OutputTrimmed()

	seedFile := filepath.Join(tmpDir, "provisioning_token_seed.yaml")
	err := os.WriteFile(seedFile, incusOSSeedFileYAMLTemplate, 0o600)
	require.NoError(t, err)

	mustRun(t, `../bin/operations-center.linux.%s provisioning token seed add %s public %s --public`, cpuArch, token, seedFile)
	mustRun(t, `../bin/operations-center.linux.%s provisioning token seed add %s private %s`, cpuArch, token, seedFile)

	// No client certificate is presented.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // nolint: gosec // self signed certificate of the Operations Center under test.
		},
	}

	seedImageURL := func(seedName string, imageType string) string {
		return fmt.Sprintf("%s/provisioning/tokens/%s/seeds/%s/architecture/%s/type/%s/image.%s", ocAPI.baseURL, token, seedName, incusOSArchitecture(t), imageType, imageType)
	}

	// The image of a seed, which is not public, is refused before it is generated.
	mustRequestMedia(t, client, http.MethodHead, seedImageURL("private", "iso"), http.Header{}, http.StatusForbidden)

	for _, imageType := range []string{"iso", "raw"} {
		// Run test
		// The first request generates the image, which takes a while.
		header, _ := mustRequestMediaWithTimeout(t, client, http.MethodHead, seedImageURL("public", imageType), http.Header{}, http.StatusOK, 10*time.Minute)

		// Assert
		require.Equal(t, "bytes", header.Get("Accept-Ranges"), "expect the %s image of a public seed to be served with support for byte ranges", imageType)

		size, err := strconv.ParseInt(header.Get("Content-Length"), 10, 64)
		require.NoError(t, err, "expect the %s image of a public seed to be served with its size", imageType)
		require.GreaterOrEqual(t, size, int64(incusOSImageMinSize), "expect the %s image of a public seed to have the size of an IncusOS image", imageType)

		// Run test
		header, body := mustRequestMedia(t, client, http.MethodGet, seedImageURL("public", imageType), http.Header{"Range": []string{"bytes=0-4095"}}, http.StatusPartialContent)

		// Assert
		require.Equal(t, fmt.Sprintf("bytes 0-4095/%d", size), header.Get("Content-Range"), "expect the requested range of the %s image to be served", imageType)
		require.Len(t, body, 4096, "expect the complete range of the %s image to be served", imageType)

		hasGPTHeader := slices.ContainsFunc(gptHeaderOffsets, func(offset int64) bool {
			return bytes.HasPrefix(body[offset:], []byte(gptHeaderMagic))
		})
		require.True(t, hasGPTHeader, "expect the %s image of a public seed to carry the GPT header of an IncusOS image at one of the offsets %v", imageType, gptHeaderOffsets)
	}
}
