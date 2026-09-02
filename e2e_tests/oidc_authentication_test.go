package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	oidcclient "github.com/FuturFusion/operations-center/internal/client/oidc"
)

const (
	oidcRemoteName       = "e2e-test-oidc"
	oidcTokenDescription = "e2e OIDC write access"
)

// oidcAuthentication verifies, that the operations-center CLI can authenticate
// against Operations Center through an OIDC provider using the device
// authorization grant, that the authenticated user is granted read and write
// access, that an expiring access token is refreshed without a new interactive
// login and that the TLS based authentication keeps working.
func oidcAuthentication(t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	// The fake OIDC provider acts as the identity provider of Operations Center.
	provider := startOIDCProvider(t)

	operationsCenterAddress := mustRun(t, `../bin/operations-center.linux.%s remote list -f json | jq -r -e '."e2e-test".addr'`, cpuArch).OutputTrimmed()
	require.NotEmpty(t, operationsCenterAddress, "Failed to determine the address of Operations Center")

	t.Cleanup(systemSecurityOIDCCleanup(t, tmpDir))
	t.Cleanup(oidcProvisioningTokenCleanup(t))

	// Setup
	t.Log("Configure the OIDC issuer of Operations Center")
	mustSetSystemSecurityOIDC(t, tmpDir, provider.Issuer, provider.ClientID, provider.ClientID, "")

	assertSystemSecurityOIDCConfig(t, provider.Issuer, provider.ClientID)

	confDir := mustCreateIsolatedCLIConfigDir(t, tmpDir)
	tokensFilename := filepath.Join(confDir, "oidc-tokens", oidcRemoteName+".json")

	// Run test
	t.Log("Verify, that the client certificate of the isolated config dir is not trusted")
	resp := runWithTimeout(t, `BROWSER=none OPERATIONS_CENTER_CONF=%[1]s ../bin/operations-center.linux.%[2]s remote add --auth-type tls --accept-certificate e2e-test-tls %[3]s`, time.Minute, confDir, cpuArch, operationsCenterAddress)
	require.NoError(t, resp.err)
	require.False(t, resp.Success(), "expect the untrusted client certificate to be rejected")
	require.Contains(t, resp.Output(), "authentication mismatch", "expect the untrusted client certificate to be rejected")

	t.Log("Add the remote using OIDC authentication")
	resp = mustRunWithTimeout(t, `BROWSER=none OPERATIONS_CENTER_CONF=%[1]s ../bin/operations-center.linux.%[2]s remote add --auth-type oidc --accept-certificate %[3]s %[4]s`, 2*time.Minute, confDir, cpuArch, oidcRemoteName, operationsCenterAddress)

	// Assertions
	require.Contains(t, resp.Output(), "URL: "+provider.Issuer+"device?user_code=", "expect the CLI to print the verification URI of the device authorization grant")
	require.Contains(t, resp.Output(), "Code: ", "expect the CLI to print the user code of the device authorization grant")

	// The operations-center CLI derives the OIDC tokens file from the default
	// remote, so the new remote needs to become the default one.
	mustRunWithTimeout(t, `OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s remote switch %s`, time.Minute, confDir, cpuArch, oidcRemoteName)

	// Run test
	t.Log("Verify write access of the OIDC authenticated user")
	mustRunWithTimeout(t, `OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s provisioning token add --description "%s" --uses 1 --lifetime 1h`, time.Minute, confDir, cpuArch, oidcTokenDescription)

	tokenResp := mustRunWithTimeout(t, `OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s provisioning token list -f json | jq -r -e '.[] | select(.description == "%s") | .uuid'`, time.Minute, confDir, cpuArch, oidcTokenDescription)
	token := tokenResp.OutputTrimmed()
	require.NotEmpty(t, token, "expect the OIDC authenticated user to be able to create a provisioning token")

	mustRunWithTimeout(t, `OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s provisioning token remove %s`, time.Minute, confDir, cpuArch, token)

	// Assertions
	t.Log("Verify the authentication protocol and read access of the OIDC authenticated user")
	mustRunWithTimeout(t, `OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s query /1.0 | jq -r -e '.metadata.auth == "oidc"'`, time.Minute, confDir, cpuArch)
	mustRunWithTimeout(t, `OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s query /1.0 | jq -r -e '.metadata.auth_methods | index("oidc") != null'`, time.Minute, confDir, cpuArch)
	mustRunWithTimeout(t, `OPERATIONS_CENTER_CONF=%[1]s ../bin/operations-center.linux.%[2]s system security show -f json | jq -r -e '.oidc.issuer == "%[3]s"'`, time.Minute, confDir, cpuArch, provider.Issuer)
	mustRunWithTimeout(t, `OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '. | length >= 1'`, time.Minute, confDir, cpuArch)

	assertOIDCTokensFile(t, tokensFilename, provider)

	// The remaining tests rely on the TLS remote, so verify it is unaffected by
	// the OIDC configuration. These commands run without OPERATIONS_CENTER_CONF
	// and therefore use the shared config dir.
	t.Log("Verify, that the TLS based authentication is unaffected")
	mustRunWithTimeout(t, `../bin/operations-center.linux.%s query /1.0 | jq -r -e '.metadata.auth == "tls"'`, time.Minute, cpuArch)
	mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '. | length >= 1'`, time.Minute, cpuArch)

	// Run test
	assertOIDCAccessTokenRefresh(t, confDir, tokensFilename, provider)

	assertOIDCClaimIsHonoured(t, tmpDir, confDir, provider)

	assertOIDCBrowserLoginRedirect(t, operationsCenterAddress, provider)
}

func assertSystemSecurityOIDCConfig(t *testing.T, issuer string, clientID string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	mustRun(t, `../bin/operations-center.linux.%[1]s system security show -f json | jq -r -e '.oidc.issuer == "%[2]s" and .oidc.client_id == "%[3]s" and .oidc.audience == "%[3]s"'`, cpuArch, issuer, clientID)
	mustRun(t, `../bin/operations-center.linux.%s system security show -f json | jq -r -e '.trusted_tls_client_cert_fingerprints | length > 0'`, cpuArch)
}

func assertOIDCTokensFile(t *testing.T, tokensFilename string, provider *oidcProvider) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	info, err := os.Stat(tokensFilename)
	require.NoError(t, err, "expect the OIDC tokens file to be written")
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "expect the OIDC tokens file to not be readable by others")

	contents, err := os.ReadFile(tokensFilename)
	require.NoError(t, err)

	oidcContext := oidcclient.OIDCContext{}
	err = json.Unmarshal(contents, &oidcContext)
	require.NoError(t, err, "expect the OIDC tokens file to be parseable")

	require.Equal(t, provider.Issuer, oidcContext.TrustTuple.Issuer)
	require.Equal(t, provider.ClientID, oidcContext.TrustTuple.ClientID)
	require.Equal(t, provider.ClientID, oidcContext.TrustTuple.Audience)
	require.NotEmpty(t, oidcContext.Tokens.AccessToken, "expect an access token")
	require.NotEmpty(t, oidcContext.Tokens.RefreshToken, "expect a refresh token, which is required to refresh the access token")

	claims := mustDecodeJWTClaims(t, oidcContext.Tokens.AccessToken)

	require.Equal(t, provider.Issuer, claims["iss"], "expect the access token to be issued by the fake OIDC provider")
	require.Equal(t, oidcSubject, claims["sub"], "expect the subject, which Operations Center uses as the username")
	require.Contains(t, claims["aud"], provider.ClientID, "expect the audience Operations Center is configured with")
}

func assertOIDCAccessTokenRefresh(t *testing.T, confDir string, tokensFilename string, provider *oidcProvider) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	require.EqualValues(t, 1, provider.DeviceAuthorizationCount(), "expect exactly one device authorization for the login")

	accessTokenBefore := mustReadOIDCAccessToken(t, tokensFilename)

	// The access token is valid for oidcAccessTokenExpiration and the CLI
	// refreshes it 15 seconds before it expires. The wait is tied to the token
	// lifetime of the fake OIDC provider, not to the speed of the VM, so it is
	// not streched.
	t.Log("Wait for the OIDC access token to become due for refresh")
	time.Sleep(oidcAccessTokenExpiration - 10*time.Second)

	mustRunWithTimeout(t, `OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s query /1.0 | jq -r -e '.metadata.auth == "oidc"'`, 2*time.Minute, confDir, cpuArch)

	accessTokenAfter := mustReadOIDCAccessToken(t, tokensFilename)

	require.NotEqual(t, accessTokenBefore, accessTokenAfter, "expect the access token to have been refreshed")
	require.EqualValues(t, 1, provider.DeviceAuthorizationCount(), "expect the refresh to not trigger a new device authorization")
}

func assertOIDCClaimIsHonoured(t *testing.T, tmpDir string, confDir string, provider *oidcProvider) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	t.Log("Configure a claim, which is not part of the access token")
	mustSetSystemSecurityOIDC(t, tmpDir, provider.Issuer, provider.ClientID, provider.ClientID, "nonexistent_claim")

	defer mustSetSystemSecurityOIDC(t, tmpDir, provider.Issuer, provider.ClientID, provider.ClientID, "")

	resp := runWithTimeout(t, `OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s query /1.0`, 2*time.Minute, confDir, cpuArch)
	require.NoError(t, resp.err)
	require.False(t, resp.Success(), "expect the authentication to fail, if the configured claim is missing")
}

func assertOIDCBrowserLoginRedirect(t *testing.T, operationsCenterAddress string, provider *oidcProvider) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	resp := mustRunWithTimeout(t, `curl -k -s -o /dev/null -w '%%{redirect_url}' %soidc/login`, time.Minute, strings.TrimSuffix(operationsCenterAddress, "/")+"/")
	require.True(t, strings.HasPrefix(resp.OutputTrimmed(), provider.Issuer+"authorize?"), "expect the login endpoint to redirect to the OIDC provider, got: %s", resp.OutputTrimmed())
}

func mustCreateIsolatedCLIConfigDir(t *testing.T, tmpDir string) string {
	t.Helper()

	confDir := filepath.Join(tmpDir, "oidc-cli-config")

	// The temporary directory is reused between runs, so start from scratch.
	err := os.RemoveAll(confDir)
	require.NoError(t, err)

	err = os.MkdirAll(confDir, 0o700)
	require.NoError(t, err)

	return confDir
}

func mustReadOIDCAccessToken(t *testing.T, tokensFilename string) string {
	t.Helper()

	contents, err := os.ReadFile(tokensFilename)
	require.NoError(t, err)

	oidcContext := oidcclient.OIDCContext{}
	err = json.Unmarshal(contents, &oidcContext)
	require.NoError(t, err)

	require.NotEmpty(t, oidcContext.Tokens.AccessToken)

	return oidcContext.Tokens.AccessToken
}

func mustDecodeJWTClaims(t *testing.T, token string) map[string]any {
	t.Helper()

	parts := strings.Split(token, ".")
	require.Len(t, parts, 3, "expect the access token to be a JWT")

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err, "expect the payload of the access token to be base64 raw URL encoded")

	claims := map[string]any{}
	err = json.Unmarshal(payload, &claims)
	require.NoError(t, err, "expect the payload of the access token to be JSON")

	return claims
}

func oidcProvisioningTokenCleanup(t *testing.T) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is already cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(60*time.Second))
		defer cancel()

		stop := timeTrack(t, "OIDC provisioning token cleanup")
		defer stop()

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -r '.[] | select(.description == "%s") | .uuid'`, cpuArch, oidcTokenDescription)
		if !resp.Success() {
			t.Log(resp.Error())
			return
		}

		for token := range strings.Lines(resp.Output()) {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}

			resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning token remove %s || true`, cpuArch, token)
			if !resp.Success() {
				t.Log(resp.Error())
			}
		}
	}
}
