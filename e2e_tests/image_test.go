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
	"go.yaml.in/yaml/v4"
)

const (
	imageSourceName = "e2e-test"
	imageSourceURL  = "https://images.linuxcontainers.org"

	// flagsImageOS is the operating system of the image, which is uploaded
	// without an incus.tar.xz, so that Operations Center has to generate the
	// metadata itself.
	flagsImageOS      = "e2etest"
	flagsImageRelease = "1"
	flagsImageVariant = "default"
	flagsImageVersion = "20260303"
	flagsImageSize    = 256 * 1024
)

// ocIncusImagesRemoteLaunchInstance exercises the Incus image and the Incus
// image source handling of Operations Center end to end:
//
//   - Upload of image versions, both with an incus.tar.xz and with the metadata
//     provided through flags.
//   - The image CRUD commands (list, show, edit, file, remove-version, remove).
//   - Syncing images from an image source and pruning them again, when they
//     stop matching the filter expression of the source or when the source is
//     removed.
//   - The public simplestreams endpoints, which are served without
//     authentication.
//   - Consumption of both a manually uploaded and a source synced image by a
//     real Incus server, which has Operations Center registered as image remote.
//
// The Incus CLI caches the simplestreams responses of a remote for an hour, so
// Operations Center is fully populated before the remote is added on the
// server. Everything asserted after that point goes through the public
// endpoints directly instead of through the server.
//
// The background task, which refreshes all the image sources, runs every 6
// hours and its first run is delayed by 10 minutes on IncusOS. Since the
// Operations Center daemon is restarted at the beginning of every end 2 end
// test, no background refresh can interfere with the assertions below.
func ocIncusImagesRemoteLaunchInstance(names []string) func(ctx context.Context, t *testing.T, tmpDir string) {
	return func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		createCluster(names)(ctx, t, tmpDir)

		// Only test on the first server
		name := names[0]

		ocHostname := prepareServerAsOCImagesClient(ctx, t, tmpDir, name)

		t.Cleanup(ocIncusImagesCleanup(t))

		imagesDir := filepath.Join(tmpDir, "images")

		downloadDir := filepath.Join(tmpDir, "image-downloads")
		err := os.RemoveAll(downloadDir)
		require.NoError(t, err)

		err = os.MkdirAll(downloadDir, 0o700)
		require.NoError(t, err)

		alpineVersion := mustDownloadAlpineImageFiles(t, imagesDir, "incus.tar.xz", "rootfs.squashfs", "disk.qcow2")

		manualName := assertIncusImageAdd(t, imagesDir, alpineVersion)
		assertIncusImageCRUD(t, tmpDir, imagesDir, downloadDir, manualName, alpineVersion)

		flagsName := assertIncusImageAddFromFlags(t, imagesDir, downloadDir)

		sourceRelease, sourceVersion := assertIncusImageSourceRefresh(t)
		sourceName := fmt.Sprintf("alpine:%s:%s:default", sourceRelease, cpuArch)

		ocImagesURL := fmt.Sprintf("https://%s:8443/incus-images", operationsCenterIPAddress(t))
		assertPublicSimplestreams(t, downloadDir, imagesDir, ocImagesURL, manualName, alpineVersion, sourceName, sourceVersion)

		assertLaunchFromOCImagesRemote(t, name, ocHostname, sourceRelease)

		assertIncusImageSourcePrune(t, tmpDir, ocImagesURL, sourceName, sourceRelease, sourceVersion)
		assertIncusImageSourceRemove(t, ocImagesURL, sourceName, sourceRelease, manualName, flagsName)
		assertIncusImageRemove(t, flagsName)
	}
}

// prepareServerAsOCImagesClient makes the given server trust the server
// certificate of Operations Center and resolve its hostname, which is what the
// server needs in order to use Operations Center as image remote. It returns
// the hostname of Operations Center.
func prepareServerAsOCImagesClient(ctx context.Context, t *testing.T, tmpDir string, name string) string {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	t.Logf("Apply operations-center server certificate to %s", name)
	resp := mustRun(t, `../bin/operations-center.linux.%s system certificate show -f json | jq -r -e '.certificate'`, cpuArch)
	ocServerCert := resp.OutputTrimmed()

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server os system security show %s:`, cpuArch, name)

	config := map[string]any{}
	err := yaml.Unmarshal([]byte(resp.OutputTrimmed()), &config)
	require.NoError(t, err)

	config["config"].(map[string]any)["custom_ca_certs"] = []string{ocServerCert}

	configBody, err := yaml.Marshal(config)
	require.NoError(t, err)

	configFilename := filepath.Join(tmpDir, fmt.Sprintf("system_security_config_%s.yaml", name))
	err = os.WriteFile(configFilename, configBody, 0o600)
	require.NoError(t, err)

	mustRun(t, `../bin/operations-center.linux.%s provisioning server os system security edit %s: < %s`, cpuArch, name, configFilename)

	mustRun(t, `incus restart %s`, name)

	t.Logf("Waiting for %s to be ready after restart with updated CA certificates", name)
	func() {
		timeoutCtx, cancel := context.WithTimeout(ctx, strechedTimeout(5*time.Minute))
		defer cancel()

		err = waitAgentRunningWithContext(timeoutCtx, t, name)
		require.NoError(t, err)

		err = waitExpectedLogWithContext(timeoutCtx, t, name, "incus-osd", "System is ready", false)
		require.NoError(t, err)
	}()

	t.Logf("Add /etc/hosts entry for OperationsCenter on %s", name)
	resp = mustRun(t, `incus exec OperationsCenter -- hostname`)
	ocHostname := resp.OutputTrimmed()
	mustRun(t, `incus exec %s -- bash -c "echo '%s	%s' >> /etc/hosts"`, name, operationsCenterIPAddress(t), ocHostname)

	return ocHostname
}

// assertIncusImageAdd uploads an image version, for which the metadata is
// provided through an incus.tar.xz, and returns the name of the image.
func assertIncusImageAdd(t *testing.T, imagesDir string, version string) string {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	// The metadata of the alpine images of images.linuxcontainers.org reports
	// "alpinelinux" as operating system, while the simplestreams product is
	// named "alpine". The manually uploaded image and the image synced from the
	// image source therefore do not collide.
	name := fmt.Sprintf("alpinelinux:edge:%s:default", cpuArch)

	t.Log("Add images to operations-center")
	mustRunWithTimeout(t, `../bin/operations-center.linux.%[1]s image incus add %[2]s/incus.tar.xz %[2]s/root.squashfs %[2]s/disk.qcow2`, 5*time.Minute, cpuArch, imagesDir)

	resp := mustRun(t, `../bin/operations-center.linux.%s image incus list`, cpuArch)
	fmt.Println(resp.Output())

	mustRun(t, `../bin/operations-center.linux.%[1]s image incus list -f json | jq -r -e '[ .[] | select(.name == "%[2]s") ] | length == 1'`, cpuArch, name)

	// The metadata is derived from incus.tar.xz, not from flags.
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '.name == "%[2]s" and .os == "alpinelinux" and .release == "edge" and .arch == "%[3]s" and .variant == "default" and (.versions | has("%[4]s"))'`, cpuArch, name, cpuArch, version)
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '.versions["%[3]s"].items | keys | sort == ["disk.qcow2","incus.tar.xz","root.squashfs"]'`, cpuArch, name, version)

	// The file types of the well known file names are recognized.
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '.versions["%[3]s"].items | .["incus.tar.xz"].ftype == "incus.tar.xz" and .["root.squashfs"].ftype == "squashfs" and .["disk.qcow2"].ftype == "disk-kvm.img"'`, cpuArch, name, version)

	// The combined hashes, which Incus derives the image fingerprints from, are
	// calculated on upload.
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '.versions["%[3]s"].items["incus.tar.xz"] | (.combined_squashfs_sha256 | length) == 64 and (.["combined_disk-kvm-img_sha256"] | length) == 64'`, cpuArch, name, version)

	// Adding the same version a second time is rejected.
	resp = run(t, `../bin/operations-center.linux.%[1]s image incus add %[2]s/incus.tar.xz %[2]s/root.squashfs`, cpuArch, imagesDir)
	require.False(t, resp.Success(), "expect adding an existing image version to fail")

	return name
}

// assertIncusImageCRUD asserts the show, edit and file commands for the given
// image.
func assertIncusImageCRUD(t *testing.T, tmpDir string, imagesDir string, downloadDir string, name string, version string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	t.Log("Assert operations-center cli image incus")

	uploadedSHA := mustSHA256(t, filepath.Join(imagesDir, "root.squashfs"))

	// The uploaded file is stored verbatim.
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '.versions["%[3]s"].items["root.squashfs"] | .sha256 == "%[4]s" and .size > 0'`, cpuArch, name, version, uploadedSHA)

	// show renders in all the supported formats.
	resp := mustRun(t, `../bin/operations-center.linux.%s image incus show %s -f yaml`, cpuArch, name)
	require.Contains(t, resp.Output(), "name: "+name)

	mustRun(t, `../bin/operations-center.linux.%s image incus show %s`, cpuArch, name)

	// Showing a non existing image fails.
	resp = run(t, `../bin/operations-center.linux.%s image incus show does-not-exist:1:%s:default`, cpuArch, cpuArch)
	require.False(t, resp.Success(), "expect showing a non existing image to fail")

	// edit reads the new state from stdin, if stdin is not a terminal.
	imagePutFilename := filepath.Join(tmpDir, "incus_image_put.yaml")
	err := os.WriteFile(imagePutFilename, []byte("---\naliases:\n  - e2e/alpine\n  - e2e/alpine/edge\ndescription: E2E test image\n"), 0o600)
	require.NoError(t, err)

	mustRun(t, `../bin/operations-center.linux.%s image incus edit %s < %s`, cpuArch, name, imagePutFilename)
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '.description == "E2E test image" and ((.aliases | sort) == ["e2e/alpine","e2e/alpine/edge"])'`, cpuArch, name)

	// file downloads a single file of an image version. The command refuses to
	// overwrite an existing target file, so the target directory is empty.
	downloaded := filepath.Join(downloadDir, "root.squashfs")
	mustRunWithTimeout(t, `../bin/operations-center.linux.%s image incus file %s %s root.squashfs %s`, 2*time.Minute, cpuArch, name, version, downloaded)
	require.Equal(t, uploadedSHA, mustSHA256(t, downloaded), "expect the downloaded file to match the uploaded file")

	// Downloading an unknown file of a known version fails.
	resp = run(t, `../bin/operations-center.linux.%s image incus file %s %s does-not-exist %s`, cpuArch, name, version, filepath.Join(downloadDir, "does-not-exist"))
	require.False(t, resp.Success(), "expect downloading a non existing file to fail")
}

// assertIncusImageAddFromFlags uploads an image version without an
// incus.tar.xz, which makes Operations Center generate the metadata tarball
// itself, and returns the name of the image.
func assertIncusImageAddFromFlags(t *testing.T, imagesDir string, downloadDir string) string {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	t.Log("Assert operations-center cli image incus add with metadata flags")

	name := fmt.Sprintf("%s:%s:%s:%s", flagsImageOS, flagsImageRelease, cpuArch, flagsImageVariant)

	payloadDir := filepath.Join(imagesDir, flagsImageOS)
	err := os.MkdirAll(payloadDir, 0o700)
	require.NoError(t, err)

	payload := filepath.Join(payloadDir, "root.squashfs")
	payloadSHA := mustWriteFileWithContent(t, payload, flagsImageSize)

	mustRunWithTimeout(t, `../bin/operations-center.linux.%[1]s image incus add %[2]s --os %[3]s --release %[4]s --arch %[5]s --variant %[6]s --image-version %[7]s`, time.Minute, cpuArch, payload, flagsImageOS, flagsImageRelease, cpuArch, flagsImageVariant, flagsImageVersion)

	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '.name == "%[2]s" and .os == "%[3]s" and .release == "%[4]s" and .arch == "%[5]s" and .variant == "%[6]s"'`, cpuArch, name, flagsImageOS, flagsImageRelease, cpuArch, flagsImageVariant)
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '.versions["%[3]s"].items["root.squashfs"] | .ftype == "squashfs" and .sha256 == "%[4]s" and .size == %[5]d'`, cpuArch, name, flagsImageVersion, payloadSHA, flagsImageSize)

	// The generated incus.tar.xz has to be a complete xz archive containing a
	// metadata.yaml, otherwise the image is unusable for Incus.
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '.versions["%[3]s"].items["incus.tar.xz"] | .ftype == "incus.tar.xz" and .size > 0 and (.sha256 | length) == 64'`, cpuArch, name, flagsImageVersion)

	metadataFile := filepath.Join(downloadDir, "incus.tar.xz")
	mustRunWithTimeout(t, `../bin/operations-center.linux.%s image incus file %s %s incus.tar.xz %s`, time.Minute, cpuArch, name, flagsImageVersion, metadataFile)
	mustRun(t, `tar -tJf %s | grep -q '^metadata.yaml$'`, metadataFile)
	mustRun(t, `tar -xJf %s -O metadata.yaml | grep -q 'serial: "%s"'`, metadataFile, flagsImageVersion)

	// Without an incus.tar.xz all the metadata flags are required.
	resp := run(t, `../bin/operations-center.linux.%s image incus add %s`, cpuArch, payload)
	require.False(t, resp.Success(), "expect add without metadata to fail")
	require.Contains(t, resp.Output(), "Either provide the image attributes")

	// The version has to look like a simplestreams serial.
	resp = run(t, `../bin/operations-center.linux.%[1]s image incus add %[2]s --os %[3]s --release %[4]s --arch %[5]s --variant %[6]s --image-version notadate`, cpuArch, payload, flagsImageOS, flagsImageRelease, cpuArch, flagsImageVariant)
	require.False(t, resp.Success(), "expect an invalid image version to be rejected")
	require.Contains(t, resp.Output(), "8 digits long date")

	// Only the architectures known to Incus are accepted.
	resp = run(t, `../bin/operations-center.linux.%[1]s image incus add %[2]s --os %[3]s --release %[4]s --arch sparc64 --variant %[5]s --image-version %[6]s`, cpuArch, payload, flagsImageOS, flagsImageRelease, flagsImageVariant, flagsImageVersion)
	require.False(t, resp.Success(), "expect an unsupported architecture to be rejected")

	return name
}

// assertIncusImageSourceRefresh creates an image source for
// images.linuxcontainers.org and syncs a single image version from it. It
// returns the release and the version identifier of the synced image.
//
// The filter expression is pinned to one product, one version and two file
// types, so that the assertions are exact and the amount of downloaded data
// stays small. The product is the most recent stable alpine release, which is
// deliberately not the "edge" release used for the manually uploaded image.
func assertIncusImageSourceRefresh(t *testing.T) (release string, version string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	t.Log("Assert operations-center cli image incus source")

	t.Cleanup(ocIncusImageSourceCleanup(t, imageSourceName))

	resp := mustRunWithTimeout(t, `curl -sf "%[1]s/streams/v1/images.json" | jq -r -e '[ .products | keys[] | select(startswith("alpine:") and endswith(":%[2]s:default")) | select(. != "alpine:edge:%[2]s:default") ] | sort | last | split(":")[1]'`, time.Minute, imageSourceURL, cpuArch)
	release = resp.OutputTrimmed()
	require.NotEmpty(t, release, "Failed to determine a stable alpine release to sync from the image source")

	name := fmt.Sprintf("alpine:%s:%s:default", release, cpuArch)

	resp = mustRunWithTimeout(t, `curl -sf "%s/streams/v1/images.json" | jq -r -e '.products."%s".versions | keys | last'`, time.Minute, imageSourceURL, name)
	version = resp.OutputTrimmed()
	require.NotEmpty(t, version, "Failed to determine the current version of %q", name)

	t.Logf("Sync image %q, version %q from %s", name, version, imageSourceURL)

	// An empty filter expression is rejected client side, so that nobody
	// accidentally mirrors a whole image server.
	resp = run(t, `../bin/operations-center.linux.%s image incus source add %s %s --filter ''`, cpuArch, imageSourceName, imageSourceURL)
	require.False(t, resp.Success(), "expect an empty filter expression to be rejected")
	require.Contains(t, resp.Output(), "Filter expression can not be empty")

	// Omitting the filter expression entirely behaves the same.
	resp = run(t, `../bin/operations-center.linux.%s image incus source add %s %s`, cpuArch, imageSourceName, imageSourceURL)
	require.False(t, resp.Success(), "expect a missing filter expression to be rejected")

	// A filter expression, which does not compile, is rejected server side.
	resp = run(t, `../bin/operations-center.linux.%s image incus source add %s %s --filter 'no_such_field == "x"'`, cpuArch, imageSourceName, imageSourceURL)
	require.False(t, resp.Success(), "expect an invalid filter expression to be rejected")
	require.Contains(t, resp.Output(), "failed to validate filter expression")

	mustRun(t, `../bin/operations-center.linux.%[1]s image incus source list -f json | jq -r -e '[ .[] | select(.name == "%[2]s") ] | length == 0'`, cpuArch, imageSourceName)

	filter := incusImageSourceFilter(name, version, `file_type in ["incus.tar.xz", "squashfs"]`)
	mustRun(t, `../bin/operations-center.linux.%s image incus source add %s %s --filter '%s'`, cpuArch, imageSourceName, imageSourceURL, filter)

	mustRun(t, `../bin/operations-center.linux.%[1]s image incus source list -f json | jq -r -e '[ .[] | select(.name == "%[2]s" and .url == "%[3]s") ] | length == 1'`, cpuArch, imageSourceName, imageSourceURL)
	// The filter expression contains double quotes, so it is passed to jq as an
	// argument instead of being interpolated into the jq program.
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus source show %[2]s -f json | jq -r -e --arg filter '%[4]s' '.name == "%[2]s" and .url == "%[3]s" and .filter_expression == $filter'`, cpuArch, imageSourceName, imageSourceURL, filter)

	resp = mustRun(t, `../bin/operations-center.linux.%s image incus source show %s -f yaml`, cpuArch, imageSourceName)
	require.Contains(t, resp.Output(), "url: "+imageSourceURL)

	// Adding the same source a second time is rejected.
	resp = run(t, `../bin/operations-center.linux.%s image incus source add %s %s --filter 'true'`, cpuArch, imageSourceName, imageSourceURL)
	require.False(t, resp.Success(), "expect a duplicate image source to be rejected")

	// A refresh is synchronous. It downloads every file matching the filter
	// expression and verifies its sha256 against the manifest of the source.
	mustRunWithTimeout(t, `../bin/operations-center.linux.%s image incus source refresh %s`, 10*time.Minute, cpuArch, imageSourceName)

	resp = mustRun(t, `../bin/operations-center.linux.%s image incus list`, cpuArch)
	fmt.Println(resp.Output())

	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '(.versions | keys) == ["%[3]s"]'`, cpuArch, name, version)
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '.versions["%[3]s"].items | keys | sort == ["incus.tar.xz","root.squashfs"]'`, cpuArch, name, version)

	// The aliases and the description are adopted from the source.
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '(.aliases | index("alpine/%[3]s")) != null'`, cpuArch, name, release)

	// A second refresh is idempotent.
	mustRunWithTimeout(t, `../bin/operations-center.linux.%s image incus source refresh %s`, 5*time.Minute, cpuArch, imageSourceName)
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '(.versions | keys) == ["%[3]s"]'`, cpuArch, name, version)

	return release, version
}

// assertPublicSimplestreams asserts the simplestreams endpoints, which
// Operations Center serves without authentication, and that the file paths they
// advertise are actually served.
func assertPublicSimplestreams(t *testing.T, downloadDir string, imagesDir string, ocImagesURL string, manualName string, manualVersion string, sourceName string, sourceVersion string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	t.Log("Assert public simplestreams endpoints")

	mustRun(t, `curl -sfk "%s/streams/v1/index.json" | jq -r -e '.format == "index:1.0" and .index.images.datatype == "image-downloads" and .index.images.path == "streams/v1/images.json" and .index.images.format == "products:1.0"'`, ocImagesURL)
	mustRun(t, `curl -sfk "%[1]s/streams/v1/index.json" | jq -r -e '.index.images.products | (index("%[2]s") != null) and (index("%[3]s") != null)'`, ocImagesURL, manualName, sourceName)

	mustRun(t, `curl -sfk "%s/streams/v1/images.json" | jq -r -e '.content_id == "images" and .datatype == "image-downloads" and .format == "products:1.0"'`, ocImagesURL)
	mustRun(t, `curl -sfk "%[1]s/streams/v1/images.json" | jq -r -e '.products["%[2]s"] | .os == "alpinelinux" and .release == "edge" and .arch == "%[3]s" and .variant == "default" and .aliases == "e2e/alpine,e2e/alpine/edge"'`, ocImagesURL, manualName, cpuArch)
	mustRun(t, `curl -sfk "%[1]s/streams/v1/images.json" | jq -r -e '.products["%[2]s"].versions["%[3]s"].items | to_entries | all(.value | (.sha256 | length) == 64 and .size > 0 and (.ftype | length) > 0 and (.path | length) > 0)'`, ocImagesURL, manualName, manualVersion)

	assertPublicSimplestreamsFile(t, downloadDir, ocImagesURL, manualName, manualVersion, "root.squashfs", mustSHA256(t, filepath.Join(imagesDir, "root.squashfs")))
	assertPublicSimplestreamsFile(t, downloadDir, ocImagesURL, sourceName, sourceVersion, "root.squashfs", "")

	// The endpoints are public, the authenticated image API is not.
	resp := run(t, `curl -sk -o /dev/null -w '%%{http_code}' "https://%s:8443/1.0/images/incus"`, operationsCenterIPAddress(t))
	require.NotEqual(t, "200", resp.OutputTrimmed(), "expect the authenticated image API to reject an unauthenticated request")
}

// assertPublicSimplestreamsFile downloads the given file through the path
// advertised for it in images.json and verifies its checksum. If wantSHA256 is
// empty, the checksum advertised in images.json is used.
func assertPublicSimplestreamsFile(t *testing.T, downloadDir string, ocImagesURL string, name string, version string, filename string, wantSHA256 string) {
	t.Helper()

	resp := mustRun(t, `curl -sfk "%[1]s/streams/v1/images.json" | jq -r -e '.products["%[2]s"].versions["%[3]s"].items["%[4]s"].path'`, ocImagesURL, name, version, filename)
	itemPath := resp.OutputTrimmed()

	if wantSHA256 == "" {
		resp = mustRun(t, `curl -sfk "%[1]s/streams/v1/images.json" | jq -r -e '.products["%[2]s"].versions["%[3]s"].items["%[4]s"].sha256'`, ocImagesURL, name, version, filename)
		wantSHA256 = resp.OutputTrimmed()
	}

	target := filepath.Join(downloadDir, fmt.Sprintf("public_%s_%s", strings.ReplaceAll(name, ":", "_"), filename))
	mustRunWithTimeout(t, `curl -sfk -o %s "%s%s"`, 5*time.Minute, target, ocImagesURL, itemPath)

	require.Equalf(t, wantSHA256, mustSHA256(t, target), "expect the path advertised for %q of image %q to serve the file", filename, name)
}

// assertLaunchFromOCImagesRemote registers Operations Center as image remote on
// the given server and launches a container from a manually uploaded and from a
// source synced image.
func assertLaunchFromOCImagesRemote(t *testing.T, name string, ocHostname string, sourceRelease string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	t.Logf("Add operations-center as image remote to %s", name)
	mustRun(t, `incus exec %s -- incus remote add operations-center-images https://%s:8443/incus-images --protocol simplestreams`, name, ocHostname)

	resp := mustRun(t, `incus exec %s -- incus image list operations-center-images:`, name)
	fmt.Println(resp.Output())

	// The operating system reported for an image is the one of the
	// simplestreams product, so the manually uploaded alpine image
	// ("alpinelinux") and the one synced from the image source ("Alpine") are
	// distinguishable.
	t.Logf("Start container from a manually uploaded image on %s", name)
	resp = mustRun(t, `incus exec %s -- incus image list operations-center-images: --format json | jq -r -e '[ .[] | select(.type == "container" and .properties.os == "alpinelinux") | .fingerprint ] | first'`, name)
	mustRunWithTimeout(t, `incus exec %s -- incus launch operations-center-images:%s a1`, 5*time.Minute, name, resp.OutputTrimmed())

	t.Logf("Start container from a source synced image on %s", name)
	resp = mustRun(t, `incus exec %[1]s -- incus image list operations-center-images: --format json | jq -r -e '[ .[] | select(.type == "container" and .properties.release == "%[2]s") | .fingerprint ] | first'`, name, sourceRelease)
	mustRunWithTimeout(t, `incus exec %s -- incus launch operations-center-images:%s a2`, 5*time.Minute, name, resp.OutputTrimmed())

	resp = mustRun(t, `incus exec %s -- incus list`, name)
	fmt.Println(resp.Output())

	mustRun(t, `incus exec %s -- incus list -f json | jq -r -e '[ .[] | select((.name == "a1" or .name == "a2") and .status == "Running") ] | length == 2'`, name)
}

// assertIncusImageSourcePrune narrows the filter expression of the image source
// and asserts, that the files, which stop matching it, are pruned.
func assertIncusImageSourcePrune(t *testing.T, tmpDir string, ocImagesURL string, sourceName string, sourceRelease string, sourceVersion string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	t.Log("Assert operations-center cli image incus source edit prunes images")

	filter := incusImageSourceFilter(sourceName, sourceVersion, `file_type == "incus.tar.xz"`)

	sourcePutFilename := filepath.Join(tmpDir, "image_source_put.yaml")
	err := os.WriteFile(sourcePutFilename, fmt.Appendf(nil, "---\nurl: %s\nfilter_expression: '%s'\n", imageSourceURL, filter), 0o600)
	require.NoError(t, err)

	mustRun(t, `../bin/operations-center.linux.%s image incus source edit %s < %s`, cpuArch, imageSourceName, sourcePutFilename)
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus source show %[2]s -f json | jq -r -e --arg filter '%[3]s' '.filter_expression == $filter'`, cpuArch, imageSourceName, filter)

	mustRunWithTimeout(t, `../bin/operations-center.linux.%s image incus source refresh %s`, 5*time.Minute, cpuArch, imageSourceName)

	// The file, which no longer matches the filter expression, is gone.
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '.versions["%[3]s"].items | keys == ["incus.tar.xz"]'`, cpuArch, sourceName, sourceVersion)

	// It is deleted from the files repository, the remaining one is not.
	resp := run(t, `incus exec OperationsCenter -- test -e /var/lib/operations-center/images/alpine/%s/%s/default/%s/root.squashfs`, sourceRelease, cpuArch, sourceVersion)
	require.False(t, resp.Success(), "expect the pruned file to be deleted from the files repository")
	mustRun(t, `incus exec OperationsCenter -- test -e /var/lib/operations-center/images/alpine/%s/%s/default/%s/incus.tar.xz`, sourceRelease, cpuArch, sourceVersion)

	// It is no longer advertised publicly.
	mustRun(t, `curl -sfk "%[1]s/streams/v1/images.json" | jq -r -e '.products["%[2]s"].versions["%[3]s"].items | has("root.squashfs") | not'`, ocImagesURL, sourceName, sourceVersion)
}

// assertIncusImageSourceRemove removes the image source and asserts, that the
// images it provided are removed with it, while the manually uploaded images
// are left alone.
func assertIncusImageSourceRemove(t *testing.T, ocImagesURL string, sourceName string, sourceRelease string, manualName string, flagsName string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	t.Log("Assert operations-center cli image incus source remove")

	mustRunWithTimeout(t, `../bin/operations-center.linux.%s image incus source remove %s`, time.Minute, cpuArch, imageSourceName)

	mustRun(t, `../bin/operations-center.linux.%[1]s image incus source list -f json | jq -r -e '[ .[] | select(.name == "%[2]s") ] | length == 0'`, cpuArch, imageSourceName)
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus list -f json | jq -r -e '[ .[] | select(.name == "%[2]s") ] | length == 0'`, cpuArch, sourceName)

	// The files of the removed images are deleted as well. Only the directory
	// of the image itself is removed, its parent directories are left behind.
	resp := run(t, `incus exec OperationsCenter -- test -e /var/lib/operations-center/images/alpine/%s/%s/default`, sourceRelease, cpuArch)
	require.False(t, resp.Success(), "expect the files of the images of the removed source to be deleted")

	// The manually uploaded images are not affected.
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus list -f json | jq -r -e '[ .[] | select(.name == "%[2]s" or .name == "%[3]s") ] | length == 2'`, cpuArch, manualName, flagsName)
	mustRun(t, `curl -sfk "%[1]s/streams/v1/index.json" | jq -r -e '.index.images.products | (index("%[2]s") == null) and (index("%[3]s") != null)'`, ocImagesURL, sourceName, manualName)

	// Operating on a removed source fails.
	resp = run(t, `../bin/operations-center.linux.%s image incus source show %s`, cpuArch, imageSourceName)
	require.False(t, resp.Success(), "expect showing a removed image source to fail")

	resp = run(t, `../bin/operations-center.linux.%s image incus source refresh %s`, cpuArch, imageSourceName)
	require.False(t, resp.Success(), "expect refreshing a removed image source to fail")
}

// assertIncusImageRemove asserts, that removing a version and removing an image
// deletes the respective files as well.
func assertIncusImageRemove(t *testing.T, name string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	t.Log("Assert operations-center cli image incus remove")

	mustRun(t, `../bin/operations-center.linux.%s image incus remove-version %s %s`, cpuArch, name, flagsImageVersion)
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus show %[2]s -f json | jq -r -e '.versions | length == 0'`, cpuArch, name)

	resp := run(t, `incus exec OperationsCenter -- test -e /var/lib/operations-center/images/%s/%s/%s/%s/%s`, flagsImageOS, flagsImageRelease, cpuArch, flagsImageVariant, flagsImageVersion)
	require.False(t, resp.Success(), "expect the files of the removed version to be deleted")

	// Removing a non existing version fails.
	resp = run(t, `../bin/operations-center.linux.%s image incus remove-version %s %s`, cpuArch, name, flagsImageVersion)
	require.False(t, resp.Success(), "expect removing a non existing image version to fail")

	mustRun(t, `../bin/operations-center.linux.%s image incus remove %s`, cpuArch, name)
	mustRun(t, `../bin/operations-center.linux.%[1]s image incus list -f json | jq -r -e '[ .[] | select(.name == "%[2]s") ] | length == 0'`, cpuArch, name)

	resp = run(t, `incus exec OperationsCenter -- test -e /var/lib/operations-center/images/%s/%s/%s/%s`, flagsImageOS, flagsImageRelease, cpuArch, flagsImageVariant)
	require.False(t, resp.Success(), "expect the files of the removed image to be deleted")

	// Removing a non existing image fails.
	resp = run(t, `../bin/operations-center.linux.%s image incus remove %s`, cpuArch, name)
	require.False(t, resp.Success(), "expect removing a non existing image to fail")
}

func incusImageSourceFilter(name string, version string, fileTypeExpression string) string {
	return fmt.Sprintf(`name == "%s" && version == "%s" && %s`, name, version, fileTypeExpression)
}

// ocIncusImagesCleanup removes all the Incus images from Operations Center.
func ocIncusImagesCleanup(t *testing.T) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		stop := timeTrack(t, "Operations Center images cleanup")
		defer stop()

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s image incus list -f json | jq -r '.[].name'`, cpuArch)
		if !resp.Success() {
			t.Error(resp.Error())
			return
		}

		for incusImage := range strings.Lines(resp.Output()) {
			incusImage = strings.TrimSpace(incusImage)
			resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s image incus remove %s`, cpuArch, incusImage)
			if !resp.Success() {
				t.Error(resp.Error())
			}
		}
	}
}

// ocIncusImageSourceCleanup removes the given Incus image source, which also
// removes all the images provided by that source.
func ocIncusImageSourceCleanup(t *testing.T, name string) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(60*time.Second))
		defer cancel()

		stop := timeTrack(t, "Operations Center image source cleanup")
		defer stop()

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%[1]s image incus source list -f json | jq -r -e '[ .[] | select(.name == "%[2]s") ] | length == 1'`, cpuArch, name)
		if !resp.Success() {
			// The image source does not exist (any more), nothing to clean up.
			return
		}

		resp = runWithContext(ctx, t, `../bin/operations-center.linux.%s image incus source remove %s`, cpuArch, name)
		if !resp.Success() {
			t.Error(resp.Error())
		}
	}
}
