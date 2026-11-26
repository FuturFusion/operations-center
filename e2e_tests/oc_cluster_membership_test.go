package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	membershipImageOS      = "e2ecluster"
	membershipImageRelease = "1"
	membershipImageVariant = "default"
	membershipImageVersion = "20260305"
	membershipImageSize    = 256 * 1024

	// membershipUnusedTokenName is the name of a member, which never joins.
	membershipUnusedTokenName = "OperationsCenterUnused"

	// moveFilesTimeout covers moving the update files of a member.
	moveFilesTimeout = 30 * time.Minute

	// ocDataDir is the data directory of Operations Center in the VMs.
	ocDataDir = "/var/lib/operations-center"
)

// ocMemberFileStores are the directories in ocDataDir with the files, which a
// single member stores.
var ocMemberFileStores = []string{"artifacts", "images", "updates"}

// ocClusterRemoveAndRejoinMember removes a member the way a user does: it
// moves the files of the member to another member first. The removed member
// is reset to a standalone server. Then it joins the cluster again.
//
// The test removes the member with the most file sets. These are usually
// update files, so the move covers large files.
func ocClusterRemoveAndRejoinMember(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	names := operationsCenterVMNames()
	members := mustTrustOperationsCenterMembers(ctx, t, tmpDir, names)

	assertClusterTokensListAndDelete(t, members[0])

	imageName, imageSHA := mustUploadMembershipImage(t, tmpDir, members[1])
	imageFile := membershipImageFile()

	resp := mustRun(t, `%s cluster members list -f json`, members[0].oc)
	removedName, err := operationsCenterMemberWithMostFileSets(resp.output.Bytes())
	require.NoError(t, err, "Failed to find the member with the most file sets")

	removed := operationsCenterMemberByName(t, members, removedName)
	remaining := otherOperationsCenterMembers(members, removed.name)
	target := remaining[0]

	movedFiles := mustOperationsCenterMemberFiles(t, removed.name)
	require.NotEmpty(t, movedFiles, "expect %s to store files", removed.name)

	t.Logf("Remove %s, which stores %d files, and move its files to %s", removed.name, len(movedFiles), target.name)

	mustRunWithTimeout(t, `%s cluster members move-files %s %s`, moveFilesTimeout, target.oc, removed.name, target.name)

	mustRun(t, `%s cluster members list -f json | jq -e '.[] | select(.name == "%s") | .file_sets == 0'`, target.oc, removed.name)
	assertOperationsCenterMemberFilesMoved(t, movedFiles, removed.name, target.name)

	imageStoredBy := mustOperationsCenterVMsWithFile(t, imageFile)
	require.Len(t, imageStoredBy, 1, "expect the image to be stored by a single member")
	require.NotEqual(t, removed.name, imageStoredBy[0], "expect the image to be moved away from %s", removed.name)

	assertImageServedByMembers(t, tmpDir, members, imageName, imageSHA)

	// The removed member is reset. It then only trusts the unix socket.
	localCLI := mustInstallOperationsCenterLocalCLI(t, removed.name)

	mustRunOperationsCenterMemberCommand(ctx, t, target, "cluster members remove", `%s cluster members remove %s`, target.oc, removed.name)

	assertOperationsCenterMemberRemoved(ctx, t, removed, remaining, localCLI)

	assertImageServedByMembers(t, tmpDir, remaining, imageName, imageSHA)

	t.Logf("Join %s to the cluster again", removed.name)

	resp = mustRun(t, `%s cluster tokens issue %s`, target.oc, removed.name)
	token := resp.OutputTrimmed()
	require.NotEmpty(t, token, "expect a join token for %s", removed.name)

	certificate := mustServedCertificate(ctx, t, removed.address)
	mustRunOperationsCenterMemberCommand(ctx, t, removed, "cluster join", `%s cluster join %s --server-name %s --server-address %s`, localCLI, token, removed.name, removed.address)

	mustWaitOperationsCenterMemberRestarted(ctx, t, removed, certificate, names)

	assertOperationsCenterClusterHealthy(ctx, t, members)

	// The join consumes the token.
	mustRun(t, `%s cluster tokens list -f json | jq -e '[ .[] | select(.server_name == "%s") ] | length == 0'`, target.oc, removed.name)

	mustRetrustOperationsCenterRemote(ctx, t)
	assertOperationsCenterMembersSelfRegistration(ctx, t, names)

	// The member, which joined again, reads the image from another member.
	require.Equal(t, imageStoredBy, mustOperationsCenterVMsWithFile(t, imageFile), "expect the image to stay on %s", imageStoredBy[0])
	assertImageServedByMembers(t, tmpDir, members, imageName, imageSHA)

	t.Logf("Remove the image through %s, which does not store it", removed.name)

	mustRun(t, `%s image incus remove %s`, removed.oc, imageName)

	for _, member := range members {
		mustRun(t, `%s image incus list -f json | jq -e '[ .[] | select(.name == "%s") ] | length == 0'`, member.oc, imageName)
	}

	require.Empty(t, mustOperationsCenterVMsWithFile(t, imageFile), "expect the image file to be deleted on %s", imageStoredBy[0])
}

// assertClusterTokensListAndDelete issues a join token, which is never used,
// and deletes it again.
func assertClusterTokensListAndDelete(t *testing.T, member operationsCenterMember) {
	t.Helper()

	t.Log("Assert the join tokens are listed and deleted")

	t.Cleanup(leftoverCleanup(t, "join token of "+membershipUnusedTokenName, `%s cluster tokens delete %s`, member.oc, membershipUnusedTokenName))

	resp := mustRun(t, `%s cluster tokens issue %s --expiry 10m`, member.oc, membershipUnusedTokenName)
	require.NotEmpty(t, resp.OutputTrimmed(), "expect a join token for %s", membershipUnusedTokenName)

	mustRun(t, `%s cluster tokens list -f json | jq -e '[ .[] | select(.server_name == "%s") ] | length == 1'`, member.oc, membershipUnusedTokenName)

	mustRun(t, `%s cluster tokens delete %s`, member.oc, membershipUnusedTokenName)

	mustRun(t, `%s cluster tokens list -f json | jq -e '[ .[] | select(.server_name == "%s") ] | length == 0'`, member.oc, membershipUnusedTokenName)
}

// membershipImageName returns the name of the image, which
// mustUploadMembershipImage uploads.
func membershipImageName() string {
	return fmt.Sprintf("%s:%s:%s:%s", membershipImageOS, membershipImageRelease, cpuArch, membershipImageVariant)
}

// membershipImageFile returns the path of the root file system of the image
// in the member, which stores it.
func membershipImageFile() string {
	return fmt.Sprintf("%s/images/%s/%s/%s/%s/%s/root.squashfs", ocDataDir, membershipImageOS, membershipImageRelease, cpuArch, membershipImageVariant, membershipImageVersion)
}

// mustUploadMembershipImage uploads an image through member. It returns the
// name of the image and the checksum of its root file system.
func mustUploadMembershipImage(t *testing.T, tmpDir string, member operationsCenterMember) (name string, sha string) {
	t.Helper()

	name = membershipImageName()

	t.Cleanup(leftoverCleanup(t, "image "+name, `../bin/operations-center.linux.%s image incus remove %s`, cpuArch, name))

	imageDir := filepath.Join(tmpDir, "oc-cluster-membership", "upload")
	require.NoError(t, os.RemoveAll(imageDir))
	require.NoError(t, os.MkdirAll(imageDir, 0o700))

	payload := filepath.Join(imageDir, "rootfs.squashfs")
	sha = mustWriteFileWithContent(t, payload, squashfsMagic, membershipImageSize)

	mustRunWithTimeout(t, `%s image incus add %s --os %s --release %s --arch %s --variant %s --image-version %s`, time.Minute, member.oc, payload, membershipImageOS, membershipImageRelease, cpuArch, membershipImageVariant, membershipImageVersion)

	return name, sha
}

// assertImageServedByMembers downloads the root file system of the image
// through every member. Members, which do not store the file, forward the
// request to the member, which stores it.
func assertImageServedByMembers(t *testing.T, tmpDir string, members []operationsCenterMember, name string, sha string) {
	t.Helper()

	for _, member := range members {
		downloadDir := filepath.Join(tmpDir, "oc-cluster-membership", "download", member.name)
		require.NoError(t, os.RemoveAll(downloadDir))
		require.NoError(t, os.MkdirAll(downloadDir, 0o700))

		downloaded := filepath.Join(downloadDir, "root.squashfs")
		mustRunWithTimeout(t, `%s image incus file %s %s root.squashfs %s`, time.Minute, member.oc, name, membershipImageVersion, downloaded)
		require.Equal(t, sha, mustSHA256(t, downloaded), "expect %s to serve the uploaded file", member.name)
	}
}

// mustOperationsCenterMemberFiles returns the checksums by file name of the
// member files stored on the VM name. The names are relative to the data
// directory of Operations Center.
func mustOperationsCenterMemberFiles(t *testing.T, name string) map[string]string {
	t.Helper()

	resp := mustRunWithTimeout(t, `incus exec %s -- sh -c 'cd %s && for store in %s; do test -d "$store" && find "$store" -type f -print0; done | xargs -0r sha256sum'`, 10*time.Minute, name, ocDataDir, strings.Join(ocMemberFileStores, " "))

	files, err := parseSHA256Sums(resp.OutputTrimmed())
	require.NoError(t, err, "Failed to parse the checksums of the member files on %s", name)

	return files
}

// assertOperationsCenterMemberFilesMoved asserts that the VM target stores
// files and the VM source no longer stores any member file.
func assertOperationsCenterMemberFilesMoved(t *testing.T, files map[string]string, source string, target string) {
	t.Helper()

	require.Empty(t, mustOperationsCenterMemberFiles(t, source), "expect %s to store no member files after the move", source)

	targetFiles := mustOperationsCenterMemberFiles(t, target)
	for name, sum := range files {
		require.Equal(t, sum, targetFiles[name], "expect %s to store %s with the same checksum", target, name)
	}
}

// assertOperationsCenterMemberRemoved waits until removed is a standalone
// server and the remaining members no longer know it.
func assertOperationsCenterMemberRemoved(ctx context.Context, t *testing.T, removed operationsCenterMember, remaining []operationsCenterMember, localCLI string) {
	t.Helper()

	t.Logf("Assert %s is removed", removed.name)

	// The removed member restarts as a standalone server.
	err := waitForCLIOutput(ctx, t, ocClusterReuseTimeout, operationsCenterStandalone, `%s cluster members list -f json`, localCLI)
	require.NoError(t, err, "expect %s to be a standalone server", removed.name)

	remainingNames := operationsCenterMemberNames(remaining)
	for _, member := range remaining {
		err := waitOperationsCenterMembers(ctx, t, member, remainingNames, ocClusterReuseTimeout)
		require.NoError(t, err, "expect %s to see the members %v", member.name, remainingNames)

		mustRun(t, `%s provisioning server list -f json | jq -e '[ .[] | select(.name == "%s") ] | length == 0'`, member.oc, removed.name)
	}
}
