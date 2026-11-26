package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	// ocMemberCLIConfigDirName is the directory in tmpDir with one isolated CLI
	// config dir per member.
	ocMemberCLIConfigDirName = "oc-member-cli-config"

	// ocClusterReuseTimeout bounds membership health checks during setup.
	ocClusterReuseTimeout = 3 * time.Minute

	// ocClusterMemberTimeout bounds each cluster-member retry operation.
	ocClusterMemberTimeout = 5 * time.Minute

	ocClusterCertificateFilename = "/var/lib/operations-center/cluster.crt"
)

// errStopRetry marks an error that retryWithContext does not retry.
var errStopRetry = errors.New("not retryable")

// operationsCenterMember is an Operations Center VM together with a CLI that
// talks to this VM directly. OperationsCenter01 is the entry point of the
// tests. It is not necessarily the leader of the cluster.
type operationsCenterMember struct {
	name string

	// address is the API address of the member as host:port.
	address string

	// confDir is the isolated CLI config dir of the member.
	confDir string

	// oc is the command prefix of the CLI.
	oc string
}

// operationsCenterClusteredVMs returns the VMs in names that store a cluster
// certificate. Operations Center creates this certificate when clustering is
// enabled.
func operationsCenterClusteredVMs(ctx context.Context, t *testing.T, names []string) ([]string, error) {
	t.Helper()

	return operationsCenterVMsWithFile(ctx, t, names, ocClusterCertificateFilename)
}

// operationsCenterVMsWithFile returns the VMs in names that store path. It
// returns an error if it can not check a VM.
func operationsCenterVMsWithFile(ctx context.Context, t *testing.T, names []string, path string) ([]string, error) {
	t.Helper()

	var found []string
	for _, name := range names {
		resp := runWithContext(ctx, t, `incus exec %s -- sh -c 'if test -e %s; then echo yes; else echo no; fi'`, name, path)
		err := fmtRunErr(resp)
		if err != nil {
			return nil, fmt.Errorf("check %s on %s: %w", path, name, err)
		}

		switch resp.OutputTrimmed() {
		case "yes":
			found = append(found, name)

		case "no":

		default:
			return nil, fmt.Errorf("check %s on %s: unexpected output %q", path, name, resp.OutputTrimmed())
		}
	}

	return found, nil
}

// mustOperationsCenterVMsWithFile returns the selected Operations Center VMs
// that store path. In a cluster, the files of a member can be on any member.
func mustOperationsCenterVMsWithFile(t *testing.T, path string) []string {
	t.Helper()

	found, err := operationsCenterVMsWithFile(t.Context(), t, operationsCenterVMNames(), path)
	require.NoError(t, err, "Failed to check the Operations Center VMs for %s", path)

	return found
}

// operationsCenterVMsNeedReinstall reports whether the VMs in names must be
// installed again. This is the case if they are clustered in a run without
// clustering, if only some of them are clustered, or if the cluster is not
// healthy.
func operationsCenterVMsNeedReinstall(ctx context.Context, t *testing.T, tmpDir string, names []string) bool {
	t.Helper()

	clustered, err := operationsCenterClusteredVMs(ctx, t, names)
	require.NoError(t, err, "Failed to check which Operations Center VMs of %v are clustered", names)

	switch {
	case !ocCluster && len(clustered) > 0:
		t.Logf("Operations Center VMs %v are clustered, installing them again for a run without clustering", clustered)

		return true

	case !ocCluster || len(clustered) == 0:
		return false

	case len(clustered) < len(names):
		t.Logf("Only the Operations Center VMs %v out of %v are clustered, installing them again", clustered, names)

		return true
	}

	healthy, err := operationsCenterClusterHealthy(ctx, t, tmpDir, names)
	require.NoError(t, err, "Failed to check the health of the Operations Center cluster %v", names)

	if !healthy {
		t.Logf("Operations Center cluster of %v is not healthy, installing it again", names)

		return true
	}

	return false
}

// operationsCenterClusterHealthy reports whether every member sees all the
// members online with the expected number of voters and a single leader. An
// unreachable member makes the cluster unhealthy. Other failures, for example
// a canceled ctx, return an error.
func operationsCenterClusterHealthy(ctx context.Context, t *testing.T, tmpDir string, names []string) (bool, error) {
	t.Helper()

	checkCtx, cancel := context.WithTimeout(ctx, strechedTimeout(ocClusterReuseTimeout))
	defer cancel()

	for _, name := range names {
		member, err := newOperationsCenterMember(checkCtx, t, tmpDir, name)
		if err != nil {
			return false, err
		}

		err = waitOperationsCenterMemberSees(checkCtx, t, member, names)
		switch {
		case err == nil:

		case ctx.Err() != nil:
			return false, fmt.Errorf("check the health of %s: %w", name, ctx.Err())

		case checkCtx.Err() == nil && errors.Is(err, errStopRetry):
			return false, err

		default:
			t.Logf("Operations Center member %s is not healthy: %v", name, err)

			return false, nil
		}
	}

	return true, nil
}

// ensureOperationsCenterCluster forms a cluster of the VMs in names with the
// CLI. It skips this step if all VMs are clustered already. The caller removes
// an unhealthy cluster first.
func ensureOperationsCenterCluster(ctx context.Context, t *testing.T, tmpDir string, names []string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	clustered, err := operationsCenterClusteredVMs(ctx, t, names)
	require.NoError(t, err, "Failed to check which Operations Center VMs of %v are clustered", names)

	if len(clustered) == len(names) {
		t.Log("Reusing the existing Operations Center cluster")
	} else {
		formOperationsCenterCluster(ctx, t, tmpDir, names)
	}

	// The members present the cluster certificate now.
	assertOperationsCenterClusterHealthy(ctx, t, mustTrustOperationsCenterMembers(ctx, t, tmpDir, names), ocClusterReuseTimeout)

	mustRetrustOperationsCenterRemote(ctx, t)
}

// formOperationsCenterCluster enables clustering on the first member and joins
// the other members one after the other.
func formOperationsCenterCluster(ctx context.Context, t *testing.T, tmpDir string, names []string) {
	t.Helper()

	first := mustTrustOperationsCenterMember(ctx, t, tmpDir, names[0])

	t.Logf("Enabling clustering on %s", first.name)

	certificate := mustServedCertificate(ctx, t, first.address)
	mustRunOperationsCenterMemberCommand(ctx, t, first, "cluster enable", `%s cluster enable --server-name %s --server-address %s`, first.oc, first.name, first.address)

	mustWaitOperationsCenterMemberRestarted(ctx, t, first, certificate, names[:1])

	members := make([]operationsCenterMember, 0, len(names))
	members = append(members, first)
	for _, name := range names[1:] {
		member := mustTrustOperationsCenterMember(ctx, t, tmpDir, name)

		t.Logf("Joining %s to the cluster", member.name)

		var token string
		err := retryWithContext(ctx, func(ctx context.Context) error {
			resp := runWithContext(ctx, t, `%s cluster tokens issue %s`, first.oc, member.name)
			token = resp.OutputTrimmed()

			return fmtRunErr(resp)
		})
		require.NoError(t, err, "Failed to issue the join token for %s", member.name)
		require.NotEmpty(t, token, "expect a join token for %s", member.name)

		certificate := mustServedCertificate(ctx, t, member.address)
		mustRunOperationsCenterMemberCommand(ctx, t, member, "cluster join", `%s cluster join %s --server-name %s --server-address %s`, member.oc, token, member.name, member.address)

		joined := names[:len(members)+1]
		mustWaitOperationsCenterMemberRestarted(ctx, t, member, certificate, joined)

		for _, other := range members {
			err := waitOperationsCenterMemberSees(ctx, t, other, joined)
			require.NoError(t, err, "expect %s to see the members %v", other.name, joined)
		}

		members = append(members, member)
	}
}

// mustRunOperationsCenterMemberCommand runs the cluster operation command on
// member. The command restarts the daemon, so it gets its own timeout.
func mustRunOperationsCenterMemberCommand(ctx context.Context, t *testing.T, member operationsCenterMember, operation string, command string, args ...any) {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, strechedTimeout(ocClusterMemberTimeout))
	resp := runWithContext(ctx, t, command, args...)
	cancel()

	require.NoError(t, fmtRunErr(resp), "Failed to run %s on %s", operation, member.name)
}

// mustWaitOperationsCenterMemberRestarted waits until enable or join restarted
// the daemon of member. The restart is complete when member presents a
// certificate other than certificate and sees the members in names.
func mustWaitOperationsCenterMemberRestarted(ctx context.Context, t *testing.T, member operationsCenterMember, certificate []byte, names []string) {
	t.Helper()

	mustWaitServedCertificate(ctx, t, member.address, func(served []byte) bool {
		return !slices.Equal(served, certificate)
	})

	err := waitOperationsCenterMemberSees(ctx, t, member, names)
	require.NoError(t, err, "expect %s to see the members %v after its restart", member.name, names)
}

// waitOperationsCenterMemberSees waits until member sees the members in names
// online with the expected number of voters and a single leader.
func waitOperationsCenterMemberSees(ctx context.Context, t *testing.T, member operationsCenterMember, names []string) error {
	t.Helper()

	// Trust the certificate again on every attempt. It can still change while
	// the daemon restarts.
	return retryWithContext(ctx, func(ctx context.Context) error {
		err := trustOperationsCenterWithContext(ctx, t, member.confDir, member.name, member.address)
		if err != nil {
			return err
		}

		return waitOperationsCenterMembers(ctx, t, member, names, 10*time.Second)
	})
}

// assertOperationsCenterClusterHealthy asserts that every member sees all the
// members online with the expected number of voters and a single leader. It
// also asserts that all members present the same certificate.
func assertOperationsCenterClusterHealthy(ctx context.Context, t *testing.T, members []operationsCenterMember, timeout time.Duration) {
	t.Helper()

	names := make([]string, 0, len(members))
	for _, member := range members {
		names = append(names, member.name)
	}

	var certificate []byte
	for _, member := range members {
		err := waitOperationsCenterMembers(ctx, t, member, names, timeout)
		require.NoError(t, err, "expect %s to see the members %v online", member.name, names)

		served := mustServedCertificate(ctx, t, member.address)
		if certificate == nil {
			certificate = served
		}

		require.Equal(t, certificate, served, "expect %s to present the cluster certificate", member.name)
	}

	resp := mustRun(t, `%s cluster members list`, members[0].oc)
	fmt.Println(resp.Output())
}

// expectedVoters returns the number of voters of a healthy cluster of n members.
func expectedVoters(n int) int {
	if n < 3 {
		return 1
	}

	return 3
}

// waitOperationsCenterMembers waits until member sees exactly the members in
// names online with the expected number of voters and a single leader.
func waitOperationsCenterMembers(ctx context.Context, t *testing.T, member operationsCenterMember, names []string, timeout time.Duration) error {
	t.Helper()

	voters := expectedVoters(len(names))

	err := waitForCLIOutput(ctx, t, timeout, func(output []byte) (bool, error) {
		return operationsCenterMembersHealthy(output, names, voters)
	}, `%s cluster members list -f json`, member.oc)
	switch {
	case err == nil:
		return nil

	case ctx.Err() != nil:
		return fmt.Errorf("wait for the members seen by %s: %w", member.name, err)

	default:
		return fmt.Errorf("%s does not see the members %v online with %d voters and a single leader: %w", member.name, names, voters, err)
	}
}

// operationsCenterMembersHealthy reports whether output of the cluster members
// list contains exactly the members in names online with voters voters and a
// single leader.
func operationsCenterMembersHealthy(output []byte, names []string, voters int) (bool, error) {
	var members []api.ClusterMember
	err := json.Unmarshal(output, &members)
	if err != nil {
		return false, fmt.Errorf("decode the cluster members: %w", err)
	}

	seen := make([]string, 0, len(members))
	seenVoters := 0
	leaders := 0
	for _, member := range members {
		if member.Offline {
			return false, nil
		}

		seen = append(seen, member.Name)

		if slices.Contains(member.Roles, "voter") {
			seenVoters++
		}

		if member.Leader {
			leaders++
		}
	}

	slices.Sort(seen)

	return slices.Equal(seen, slices.Sorted(slices.Values(names))) && seenVoters == voters && leaders == 1, nil
}

// waitForCLIOutput runs command until check accepts its output. It retries
// failed commands and output that check rejects. It stops at once if the
// command does not start or if check returns an error. These errors wrap
// errStopRetry. On timeout, the error contains the last result.
func waitForCLIOutput(ctx context.Context, t *testing.T, timeout time.Duration, check func(output []byte) (bool, error), command string, args ...any) error {
	t.Helper()

	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, strechedTimeout(timeout))
	defer cancel()

	var last string
	for {
		resp := runQuietWithContext(ctx, t, command, args...)
		if resp.err != nil {
			return fmt.Errorf("%w: %w", errStopRetry, resp.err)
		}

		if resp.Success() {
			ok, err := check(resp.output.Bytes())
			if err != nil {
				return fmt.Errorf("%w: %w:\n%s", errStopRetry, err, resp.Output())
			}

			if ok {
				return nil
			}

			last = resp.Output()
		} else {
			last = fmtRunErr(resp).Error()
		}

		select {
		case <-ctx.Done():
			if parent.Err() != nil {
				return parent.Err()
			}

			return fmt.Errorf("%w, last result:\n%s", ctx.Err(), last)

		case <-time.After(time.Second):
		}
	}
}

// mustTrustOperationsCenterMembers is mustTrustOperationsCenterMember for
// every member in names.
func mustTrustOperationsCenterMembers(ctx context.Context, t *testing.T, tmpDir string, names []string) []operationsCenterMember {
	t.Helper()

	members := make([]operationsCenterMember, 0, len(names))
	for _, name := range names {
		members = append(members, mustTrustOperationsCenterMember(ctx, t, tmpDir, name))
	}

	return members
}

// mustTrustOperationsCenterMember configures the isolated CLI config dir of the
// member name, so it trusts the certificate the member presents now.
func mustTrustOperationsCenterMember(ctx context.Context, t *testing.T, tmpDir string, name string) operationsCenterMember {
	t.Helper()

	member, err := trustOperationsCenterMemberWithContext(ctx, t, tmpDir, name)
	require.NoError(t, err, "Failed to add the remote of %s", name)

	return member
}

// trustOperationsCenterMemberWithContext is the same as
// mustTrustOperationsCenterMember, but it returns the error instead of failing
// the test.
func trustOperationsCenterMemberWithContext(ctx context.Context, t *testing.T, tmpDir string, name string) (operationsCenterMember, error) {
	t.Helper()

	member, err := newOperationsCenterMember(ctx, t, tmpDir, name)
	if err != nil {
		return operationsCenterMember{}, err
	}

	// The remote add fails while the daemon restarts. Retry it.
	err = retryWithContext(ctx, func(ctx context.Context) error {
		return trustOperationsCenterWithContext(ctx, t, member.confDir, member.name, member.address)
	})
	if err != nil {
		return operationsCenterMember{}, err
	}

	return member, nil
}

// newOperationsCenterMember prepares the isolated CLI config dir of the member
// name. It does not add the remote of the member.
func newOperationsCenterMember(ctx context.Context, t *testing.T, tmpDir string, name string) (operationsCenterMember, error) {
	t.Helper()

	confDir := filepath.Join(tmpDir, ocMemberCLIConfigDirName, name)
	err := os.MkdirAll(confDir, 0o700)
	if err != nil {
		return operationsCenterMember{}, err
	}

	address, err := instanceIPAddressWithContext(ctx, t, name)
	if err != nil {
		return operationsCenterMember{}, err
	}

	err = copyIncusClientCertificate(ctx, t, confDir)
	if err != nil {
		return operationsCenterMember{}, err
	}

	return operationsCenterMember{
		name:    name,
		address: net.JoinHostPort(address, "8443"),
		confDir: confDir,
		oc:      operationsCenterCLI(confDir),
	}, nil
}

// operationsCenterCLI returns the command prefix to run the CLI with the
// isolated CLI config dir confDir.
func operationsCenterCLI(confDir string) string {
	return fmt.Sprintf("OPERATIONS_CENTER_CONF=%s ../bin/operations-center.linux.%s", confDir, cpuArch)
}

// copyIncusClientCertificate copies the client certificate of incus to the
// isolated CLI config dir confDir. The seed trusts this certificate.
func copyIncusClientCertificate(ctx context.Context, t *testing.T, confDir string) error {
	t.Helper()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	return fmtRunErr(runWithContext(ctx, t, `cp %s/.config/incus/client.crt %s/.config/incus/client.key %s/`, homeDir, homeDir, confDir))
}

// trustOperationsCenterWithContext adds remote to the isolated CLI config dir
// confDir. The remote points to hostPort and trusts the certificate that
// hostPort presents now. It replaces an existing remote with the same name.
func trustOperationsCenterWithContext(ctx context.Context, t *testing.T, confDir string, remote string, hostPort string) error {
	t.Helper()

	oc := operationsCenterCLI(confDir)

	// Skip the trust check, which warns on stderr once the certificate changed.
	resp := runWithContext(ctx, t, `%s remote list -f json --force-local`, oc)
	err := fmtRunErr(resp)
	if err != nil {
		return err
	}

	var remotes map[string]json.RawMessage
	err = json.Unmarshal(resp.output.Bytes(), &remotes)
	if err != nil {
		return fmt.Errorf("decode the remotes of %s: %w", confDir, err)
	}

	commands := []string{
		`%[1]s remote add --accept-certificate %[2]s https://%[3]s/`,
		`%[1]s remote switch %[2]s`,
	}

	_, ok := remotes[remote]
	if ok {
		commands = slices.Insert(commands, 0, `%[1]s remote remove %[2]s`)
	}

	for _, command := range commands {
		err = fmtRunErr(runWithContext(ctx, t, command, oc, remote, hostPort))
		if err != nil {
			return err
		}
	}

	return nil
}

// mustServedCertificate returns the DER bytes of the leaf certificate that
// hostPort presents.
func mustServedCertificate(ctx context.Context, t *testing.T, hostPort string) []byte {
	t.Helper()

	chain, err := servedCertificate(ctx, hostPort)
	require.NoError(t, err, "Failed to get the certificate of %s", hostPort)

	return chain[0].Raw
}

// mustWaitServedCertificate waits until hostPort presents a leaf certificate
// that satisfies want.
func mustWaitServedCertificate(ctx context.Context, t *testing.T, hostPort string, want func(served []byte) bool) {
	t.Helper()

	err := retryWithContext(ctx, func(ctx context.Context) error {
		chain, err := servedCertificate(ctx, hostPort)
		if err != nil {
			return err
		}

		if !want(chain[0].Raw) {
			return fmt.Errorf("%s does not present the expected certificate yet", hostPort)
		}

		return nil
	})
	require.NoError(t, err, "expect %s to present the expected certificate", hostPort)
}

// retryWithContext retries f after a one-second delay until success, context
// cancellation, or the stretched ocClusterMemberTimeout. It does not retry an
// error that wraps errStopRetry.
func retryWithContext(ctx context.Context, f func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, strechedTimeout(ocClusterMemberTimeout))
	defer cancel()

	for {
		err := f(ctx)
		if err == nil || errors.Is(err, errStopRetry) {
			return err
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", ctx.Err(), err)

		case <-time.After(time.Second):
		}
	}
}
