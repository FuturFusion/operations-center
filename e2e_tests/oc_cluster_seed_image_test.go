package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const seedImageAnyMemberTokenDescription = "e2e-seed-image-any-member"

// ocClusterSeedImageThroughAnyMember requests the image of a public token seed
// through every member. To build the image, a member needs the files of an
// update. Only one member stores them, so the other members read them from
// this member.
func ocClusterSeedImageThroughAnyMember(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	members := mustTrustOperationsCenterMembers(ctx, t, tmpDir, operationsCenterVMNames())

	t.Cleanup(provisioningTokensCleanup(t, seedImageAnyMemberTokenDescription))

	mustRun(t, `%s provisioning token add --description %s --uses 1 --lifetime 1h`, members[0].oc, seedImageAnyMemberTokenDescription)
	token := mustRun(t, `%s provisioning token list -f json | jq -r -e '.[] | select(.description == "%s") | .uuid'`, members[1].oc, seedImageAnyMemberTokenDescription).OutputTrimmed()

	seedFile := filepath.Join(tmpDir, "oc_cluster_seed_image.yaml")
	err := os.WriteFile(seedFile, incusOSSeedFileYAMLTemplate, 0o600)
	require.NoError(t, err)

	mustRun(t, `%s provisioning token seed add %s public %s --public`, members[1].oc, token, seedFile)

	// No client certificate is presented, like a server does during its installation.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // nolint: gosec // self signed certificate of the Operations Center under test.
		},
	}

	for _, member := range members {
		t.Logf("Assert %s serves the image of the public token seed", member.name)

		url := fmt.Sprintf("https://%s/1.0/provisioning/tokens/%s/seeds/public/architecture/%s/type/iso/image.iso", member.address, token, incusOSArchitecture(t))
		assertPublicSeedImageServed(t, client, url, "iso")
	}
}
