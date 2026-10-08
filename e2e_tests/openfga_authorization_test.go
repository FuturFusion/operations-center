package e2e

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	openFGAConfDirName      = "openfga-cli-config"
	openFGATokenDescription = "e2e OpenFGA authorization"
)

type assertCmd func(t *testing.T, resp cmdResponse, msgAndArgs ...any)

func assertCmdAllowed(t *testing.T, resp cmdResponse, msgAndArgs ...any) {
	t.Helper()

	require.NoError(t, fmtRunErr(resp), msgAndArgs...)
}

func assertCmdDenied(entitlement string) assertCmd {
	return func(t *testing.T, resp cmdResponse, msgAndArgs ...any) {
		t.Helper()

		require.NoError(t, resp.err, msgAndArgs...)
		require.False(t, resp.Success(), msgAndArgs...)
		require.Contains(t, resp.Output(), fmt.Sprintf("User does not have entitlement %q", entitlement), msgAndArgs...)
	}
}

// openFGAAuthorization verifies the OpenFGA authorization of OIDC users. Each
// role of the built-in authorization model grants the expected entitlements.
// TLS authentication keeps its unrestricted access. Operations Center rejects
// an unreachable OpenFGA server.
func openFGAAuthorization(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	// Setup
	provider := startOIDCProvider(t)
	backend := startOpenFGA(t)

	operationsCenterAddress := mustRun(t, `../bin/operations-center.linux.%s remote list -f json | jq -r -e '."e2e-test".addr'`, cpuArch).OutputTrimmed()
	require.NotEmpty(t, operationsCenterAddress, "Failed to determine the address of Operations Center")

	t.Cleanup(systemSecurityOIDCAndOpenFGACleanup(t, tmpDir))
	t.Cleanup(provisioningTokenCleanup(t, openFGATokenDescription))

	// A failed run without cleanup leaves its security config behind.
	mustResetSystemSecurityOIDCAndOpenFGA(ctx, t, tmpDir)

	t.Log("Configure the OIDC issuer and the OpenFGA server of Operations Center")
	mustSetSystemSecurityOIDC(ctx, t, tmpDir, provider.Issuer, provider.ClientID, provider.ClientID, "")
	mustSetSystemSecurityOpenFGA(ctx, t, tmpDir, backend.APIURL, openFGAAPIToken, backend.StoreID)

	assertSystemSecurityOpenFGAConfig(t, backend.APIURL, backend.StoreID)

	backend.mustAssertInitialized(t)

	confDir := mustCreateIsolatedCLIConfigDir(t, tmpDir, openFGAConfDirName)

	_ = mustOIDCLogin(t, confDir, operationsCenterAddress)

	tests := []struct {
		name string
		// role is the relation of the OIDC user on the Operations Center server
		// object.
		role string

		assertInfo   assertCmd
		assertView   assertCmd
		assertEdit   assertCmd
		assertCreate assertCmd
		assertDelete assertCmd
	}{
		{
			name: "no role",
			role: "",

			assertInfo:   assertCmdAllowed,
			assertView:   assertCmdDenied("can_view"),
			assertEdit:   assertCmdDenied("can_edit"),
			assertCreate: assertCmdDenied("can_create"),
			assertDelete: assertCmdDenied("can_delete"),
		},
		{
			name: "viewer",
			role: "viewer",

			assertInfo:   assertCmdAllowed,
			assertView:   assertCmdAllowed,
			assertEdit:   assertCmdDenied("can_edit"),
			assertCreate: assertCmdDenied("can_create"),
			assertDelete: assertCmdDenied("can_delete"),
		},
		{
			name: "user",
			role: "user",

			assertInfo:   assertCmdAllowed,
			assertView:   assertCmdAllowed,
			assertEdit:   assertCmdDenied("can_edit"),
			assertCreate: assertCmdDenied("can_create"),
			assertDelete: assertCmdDenied("can_delete"),
		},
		{
			name: "operator",
			role: "operator",

			assertInfo:   assertCmdAllowed,
			assertView:   assertCmdAllowed,
			assertEdit:   assertCmdAllowed,
			assertCreate: assertCmdDenied("can_create"),
			assertDelete: assertCmdDenied("can_delete"),
		},
		{
			name: "admin",
			role: "admin",

			assertInfo:   assertCmdAllowed,
			assertView:   assertCmdAllowed,
			assertEdit:   assertCmdAllowed,
			assertCreate: assertCmdAllowed,
			assertDelete: assertCmdAllowed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stop := timeTrack(t, "role "+tc.name)
			defer stop()

			// Setup
			backend.mustSetRole(t, oidcSubject, tc.role)

			// Create the token through the TLS remote. The edit and the remove
			// command need a target for every role.
			token := mustCreateOpenFGATargetToken(t, tmpDir)

			// Run test
			infoResp := runWithTimeout(t, `OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s query /1.0 | jq -r -e '.metadata.auth == "oidc"'`, time.Minute, confDir, cpuArch)
			viewResp := runWithTimeout(t, `OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s provisioning token list -f json`, time.Minute, confDir, cpuArch)
			editResp := runWithTimeout(t, `OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s provisioning token edit %s < %s`, time.Minute, confDir, cpuArch, token.uuid, token.putFilename)
			createResp := runWithTimeout(t, `OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s provisioning token add --description "%s" --uses 1 --lifetime 1h`, time.Minute, confDir, cpuArch, openFGATokenDescription)
			deleteResp := runWithTimeout(t, `OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s provisioning token remove %s`, time.Minute, confDir, cpuArch, token.uuid)

			// Assertions
			tc.assertInfo(t, infoResp, "role %q: query /1.0", tc.role)
			tc.assertView(t, viewResp, "role %q: provisioning token list (can_view)", tc.role)
			tc.assertEdit(t, editResp, "role %q: provisioning token edit (can_edit)", tc.role)
			tc.assertCreate(t, createResp, "role %q: provisioning token add (can_create)", tc.role)
			tc.assertDelete(t, deleteResp, "role %q: provisioning token remove (can_delete)", tc.role)
		})
	}

	// Run test
	t.Log("Verify that the TLS based authentication keeps its unrestricted access")
	mustRunWithTimeout(t, `../bin/operations-center.linux.%s query /1.0 | jq -r -e '.metadata.auth == "tls"'`, time.Minute, cpuArch)

	token := mustCreateOpenFGATargetToken(t, tmpDir)
	mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning token remove %s`, time.Minute, cpuArch, token.uuid)

	assertUnreachableOpenFGAIsRejected(ctx, t, tmpDir, backend)
}

func assertSystemSecurityOpenFGAConfig(t *testing.T, apiURL string, storeID string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	mustRun(t, `../bin/operations-center.linux.%s system security show -f json | jq -r -e '.openfga.api_url == "%s" and .openfga.store_id == "%s"'`, cpuArch, apiURL, storeID)
	mustRun(t, `../bin/operations-center.linux.%s system security show -f json | jq -r -e '.trusted_tls_client_cert_fingerprints | length > 0'`, cpuArch)
}

func assertUnreachableOpenFGAIsRejected(ctx context.Context, t *testing.T, tmpDir string, backend *openFGABackend) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	// Setup
	// Listen and close again to get a port without a listener.
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err, "Failed to listen on a free port")

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok, "Failed to get the port of the listener")

	err = listener.Close()
	require.NoError(t, err, "Failed to close the listener")

	unreachableAPIURL := "http://" + net.JoinHostPort(e2eHostAddress(t), strconv.Itoa(tcpAddr.Port))

	// Run test
	t.Log("Verify that Operations Center rejects an unreachable OpenFGA server")
	err = setSystemSecurityOpenFGAWithContext(ctx, t, tmpDir, unreachableAPIURL, openFGAAPIToken, backend.StoreID)

	// Assertions
	require.ErrorContains(t, err, "Failed to reach OpenFGA", "expect Operations Center to reject an OpenFGA server it cannot reach")

	assertSystemSecurityOpenFGAConfig(t, backend.APIURL, backend.StoreID)
}

type openFGATargetToken struct {
	uuid string
	// putFilename holds the current properties of the token. It is the input
	// for "provisioning token edit".
	putFilename string
}

// mustCreateOpenFGATargetToken creates a provisioning token through the TLS
// remote. It also writes the input file for "provisioning token edit".
func mustCreateOpenFGATargetToken(t *testing.T, tmpDir string) openFGATargetToken {
	t.Helper()

	before := mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -c '[ .[].uuid ]'`, time.Minute, cpuArch).OutputTrimmed()

	mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning token add --description "%s" --uses 1 --lifetime 1h`, time.Minute, cpuArch, openFGATokenDescription)

	uuid := mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -r -e --argjson before '%s' '[ .[] | select(.description == "%s") | .uuid ] - $before | first'`, time.Minute, cpuArch, before, openFGATokenDescription).OutputTrimmed()
	require.NotEmpty(t, uuid, "Failed to determine the UUID of the created provisioning token")

	putFilename := filepath.Join(tmpDir, "openfga_token_put.json")

	mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning token list -f json | jq -c -e '.[] | select(.uuid == "%s") | del(.uuid)' > %s`, time.Minute, cpuArch, uuid, putFilename)

	return openFGATargetToken{
		uuid:        uuid,
		putFilename: putFilename,
	}
}
