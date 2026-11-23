package cluster

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
	"github.com/FuturFusion/operations-center/internal/warning"
	warningMock "github.com/FuturFusion/operations-center/internal/warning/mock"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

// Not parallel, since it changes the global configuration.
func TestMembership_prepareServerCertificate_acmeFailure(t *testing.T) {
	dir := t.TempDir()
	env := &envMock.EnvironmentMock{
		VarDirFunc:    func() string { return dir },
		CacheDirFunc:  func() string { return dir },
		IsIncusOSFunc: func() bool { return false },
		ClusteredFunc: func() bool { return false },
	}

	config.InitTest(t, env, nil)

	security := config.GetSecurity().SecurityPut
	security.ACME = system.SecurityACME{
		Domain:   "oc.example.com",
		Email:    "admin@example.com",
		CAURL:    "http://127.0.0.1:1/directory",
		AgreeTOS: true,
	}

	require.NoError(t, config.UpdateSecurity(t.Context(), security))

	var emitted []warning.Warning
	m := &membership{
		env:     env,
		varRoot: fsroot.Open(t, dir),
		warnings: &warningMock.WarningServiceMock{
			EmitFunc: func(ctx context.Context, w warning.Warning) {
				emitted = append(emitted, w)
			},
		},
	}

	// The certificate files are missing, the ACME request fails before they are read.
	_, _ = m.prepareServerCertificate(t.Context(), true)

	require.Len(t, emitted, 1, "the self-signed fallback is reported as a warning")
	require.Equal(t, api.WarningTypeACMECertificateUpdateFailed, emitted[0].Type)
	scope := api.WarningScope{Scope: emitted[0].Scope, EntityType: emitted[0].EntityType, Entity: emitted[0].Entity}
	require.Equal(t, warning.ScopeACMECertificateUpdate, scope)
}
