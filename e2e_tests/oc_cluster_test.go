package e2e

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_operationsCenterMembersHealthy(t *testing.T) {
	tests := []struct {
		name   string
		output string
		names  []string
		voters int

		assertErr   require.ErrorAssertionFunc
		wantHealthy bool
	}{
		{
			name:   "healthy cluster of three",
			output: `[{"name":"b","roles":["voter"]},{"name":"a","roles":["voter"],"leader":true},{"name":"c","roles":["voter"]}]`,
			names:  []string{"a", "b", "c"},
			voters: 3,

			assertErr:   require.NoError,
			wantHealthy: true,
		},
		{
			name:   "member offline",
			output: `[{"name":"a","roles":["voter"],"leader":true},{"name":"b","roles":["voter"],"offline":true}]`,
			names:  []string{"a", "b"},
			voters: 2,

			assertErr:   require.NoError,
			wantHealthy: false,
		},
		{
			name:   "member missing",
			output: `[{"name":"a","roles":["voter"],"leader":true}]`,
			names:  []string{"a", "b"},
			voters: 1,

			assertErr:   require.NoError,
			wantHealthy: false,
		},
		{
			name:   "wrong number of voters",
			output: `[{"name":"a","roles":["voter"],"leader":true},{"name":"b","roles":["spare"]}]`,
			names:  []string{"a", "b"},
			voters: 2,

			assertErr:   require.NoError,
			wantHealthy: false,
		},
		{
			name:   "no leader",
			output: `[{"name":"a","roles":["voter"]}]`,
			names:  []string{"a"},
			voters: 1,

			assertErr:   require.NoError,
			wantHealthy: false,
		},
		{
			name:   "invalid output",
			output: `not json`,
			names:  []string{"a"},
			voters: 1,

			assertErr:   require.Error,
			wantHealthy: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			healthy, err := operationsCenterMembersHealthy([]byte(tc.output), tc.names, tc.voters)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantHealthy, healthy, "expect the health of the members")
		})
	}
}

func Test_operationsCenterServersRegistered(t *testing.T) {
	tests := []struct {
		name   string
		output string
		names  []string

		assertErr      require.ErrorAssertionFunc
		wantRegistered bool
	}{
		{
			name:   "all members registered",
			output: `[{"name":"b","server_type":"operations-center"},{"name":"x","server_type":"incus"},{"name":"a","server_type":"operations-center"}]`,
			names:  []string{"a", "b"},

			assertErr:      require.NoError,
			wantRegistered: true,
		},
		{
			name:   "member missing",
			output: `[{"name":"a","server_type":"operations-center"},{"name":"b","server_type":"incus"}]`,
			names:  []string{"a", "b"},

			assertErr:      require.NoError,
			wantRegistered: false,
		},
		{
			name:   "invalid output",
			output: `not json`,
			names:  []string{"a"},

			assertErr:      require.Error,
			wantRegistered: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			registered, err := operationsCenterServersRegistered([]byte(tc.output), tc.names)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantRegistered, registered, "expect the registration of the members")
		})
	}
}

func Test_waitForCLIOutput(t *testing.T) {
	tests := []struct {
		name    string
		command string
		check   func(output []byte) (bool, error)

		assertErr     require.ErrorAssertionFunc
		wantStopRetry bool
	}{
		{
			name:    "accepted output",
			command: `echo ok`,
			check: func(output []byte) (bool, error) {
				return string(output) == "ok\n", nil
			},

			assertErr:     require.NoError,
			wantStopRetry: false,
		},
		{
			name:    "rejected output until timeout",
			command: `echo no`,
			check: func(output []byte) (bool, error) {
				return false, nil
			},

			assertErr:     require.Error,
			wantStopRetry: false,
		},
		{
			name:    "failed command until timeout",
			command: `false`,
			check: func(output []byte) (bool, error) {
				return true, nil
			},

			assertErr:     require.Error,
			wantStopRetry: false,
		},
		{
			name:    "check error stops at once",
			command: `echo broken`,
			check: func(output []byte) (bool, error) {
				return false, errStopRetry
			},

			assertErr:     require.Error,
			wantStopRetry: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := waitForCLIOutput(t.Context(), t, 500*time.Millisecond, tc.check, `%s`, tc.command)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantStopRetry, errors.Is(err, errStopRetry), "expect the error to stop a retry")
		})
	}
}

func Test_operationsCenterLeader(t *testing.T) {
	tests := []struct {
		name   string
		output string

		assertErr  require.ErrorAssertionFunc
		wantLeader string
	}{
		{
			name:   "single leader",
			output: `[{"name":"a","roles":["voter"]},{"name":"b","roles":["voter"],"leader":true}]`,

			assertErr:  require.NoError,
			wantLeader: "b",
		},
		{
			name:   "no leader",
			output: `[{"name":"a","roles":["voter"]},{"name":"b","roles":["voter"]}]`,

			assertErr:  require.NoError,
			wantLeader: "",
		},
		{
			name:   "two leaders",
			output: `[{"name":"a","roles":["voter"],"leader":true},{"name":"b","roles":["voter"],"leader":true}]`,

			assertErr:  require.NoError,
			wantLeader: "",
		},
		{
			name:   "invalid output",
			output: `not json`,

			assertErr:  require.Error,
			wantLeader: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			leader, err := operationsCenterLeader([]byte(tc.output))

			tc.assertErr(t, err)
			require.Equal(t, tc.wantLeader, leader, "expect the leader of the members")
		})
	}
}

func Test_otherOperationsCenterMembers(t *testing.T) {
	members := []operationsCenterMember{{name: "a"}, {name: "b"}, {name: "c"}}

	tests := []struct {
		name string
		skip string

		wantNames []string
	}{
		{
			name: "first member",
			skip: "a",

			wantNames: []string{"b", "c"},
		},
		{
			name: "middle member",
			skip: "b",

			wantNames: []string{"a", "c"},
		},
		{
			name: "unknown member",
			skip: "x",

			wantNames: []string{"a", "b", "c"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			others := otherOperationsCenterMembers(members, tc.skip)

			require.Equal(t, tc.wantNames, operationsCenterMemberNames(others), "expect the other members")
			require.Equal(t, []string{"a", "b", "c"}, operationsCenterMemberNames(members), "expect the members to be unchanged")
		})
	}
}
