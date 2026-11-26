package memberfiles_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

// moverCluster is a cluster whose members are served by test servers.
type moverCluster struct {
	members map[int64]cowsqldb.NodeInfo
	urls    map[string]string
}

func (c moverCluster) LocalMemberID() (int64, error) { return 1, nil }

func (c moverCluster) GetMembersByID(context.Context) (map[int64]cowsqldb.NodeInfo, error) {
	return c.members, nil
}

func (c moverCluster) HeartbeatOfflineThreshold() time.Duration { return 20 * time.Second }

func (c moverCluster) MemberClient(address string) (client.OperationsCenterClient, error) {
	return client.New(c.urls[address])
}

// fileTargets records the member storing each unit.
type fileTargets struct {
	mu        sync.Mutex
	units     []memberfiles.Unit
	targets   map[string]int64
	setErrFor string
}

func (f *fileTargets) GetUnitsByTarget(_ context.Context, memberID int64) ([]memberfiles.Unit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var units []memberfiles.Unit
	for _, unit := range f.units {
		if f.targets[unit.Name] == memberID {
			units = append(units, unit)
		}
	}

	return units, nil
}

func (f *fileTargets) SetUnitTarget(_ context.Context, unit memberfiles.Unit, memberID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if unit.Name == f.setErrFor {
		return boom.Error
	}

	f.targets[unit.Name] = memberID

	return nil
}

// fileRequests records the file requests a member got, and fails the ones for the unit failFor.
type fileRequests struct {
	mu       sync.Mutex
	requests []string
	failFor  string
}

func (f *fileRequests) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+strings.TrimPrefix(r.URL.Path, "/internal/files/"))
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	if f.failFor != "" && strings.HasSuffix(r.URL.Path, "/"+f.failFor) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":"Failed","error_code":500}`))

		return
	}

	_, _ = w.Write([]byte(`{"type":"sync","status":"Success","status_code":200,"metadata":{}}`))
}

func TestMover_Move(t *testing.T) {
	t.Parallel()

	const sourceID, targetID = 2, 3

	tests := []struct {
		name        string
		targetName  string
		targetBeat  time.Time
		pullFailFor string
		delFailFor  string
		setErrFor   string

		wantErr     require.ErrorAssertionFunc
		wantTargets map[string]int64
		wantPulls   []string
		wantDeletes []string
	}{
		{
			name:       "all units move",
			targetName: "member03",
			targetBeat: time.Now(),

			wantErr:     require.NoError,
			wantTargets: map[string]int64{"unit-a": targetID, "unit-b": targetID},
			wantPulls:   []string{"POST images/unit-a", "POST images/unit-b"},
			wantDeletes: []string{"DELETE images/unit-a", "DELETE images/unit-b"},
		},
		{
			name:        "a unit failing to copy is kept on the source",
			targetName:  "member03",
			targetBeat:  time.Now(),
			pullFailFor: "unit-a",

			wantErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrUnavailable, a...)
				require.ErrorContains(tt, err, `Failed to move 1 file sets from cluster member "member02" to "member03"`, a...)
			},
			wantTargets: map[string]int64{"unit-a": sourceID, "unit-b": targetID},
			wantPulls:   []string{"POST images/unit-a", "POST images/unit-b"},
			wantDeletes: []string{"DELETE images/unit-b"},
		},
		{
			name:       "a unit failing to record its target is kept on the source",
			targetName: "member03",
			targetBeat: time.Now(),
			setErrFor:  "unit-b",

			wantErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrUnavailable, a...)
				require.ErrorIs(tt, err, boom.Error, a...)
			},
			wantTargets: map[string]int64{"unit-a": targetID, "unit-b": sourceID},
			wantPulls:   []string{"POST images/unit-a", "POST images/unit-b"},
			wantDeletes: []string{"DELETE images/unit-a"},
		},
		{
			name:       "a copy failing to delete on the source still moves",
			targetName: "member03",
			targetBeat: time.Now(),
			delFailFor: "unit-a",

			wantErr:     require.NoError,
			wantTargets: map[string]int64{"unit-a": targetID, "unit-b": targetID},
			wantPulls:   []string{"POST images/unit-a", "POST images/unit-b"},
			wantDeletes: []string{"DELETE images/unit-a", "DELETE images/unit-b"},
		},
		{
			name:       "an offline target gets no files",
			targetName: "member03",
			targetBeat: time.Now().Add(-time.Hour),

			wantErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrUnavailable, a...)
				require.ErrorContains(tt, err, `Cluster member "member03" is offline`, a...)
			},
			wantTargets: map[string]int64{"unit-a": sourceID, "unit-b": sourceID},
			wantPulls:   nil,
			wantDeletes: nil,
		},
		{
			name:       "an unknown target gets no files",
			targetName: "member09",
			targetBeat: time.Now(),

			wantErr:     errassert.NotFoundError,
			wantTargets: map[string]int64{"unit-a": sourceID, "unit-b": sourceID},
			wantPulls:   nil,
			wantDeletes: nil,
		},
		{
			name:       "the source as target is invalid",
			targetName: "member02",
			targetBeat: time.Now(),

			wantErr: func(tt require.TestingT, err error, a ...any) {
				var validationErr domain.ErrValidation
				require.ErrorAs(tt, err, &validationErr, a...)
			},
			wantTargets: map[string]int64{"unit-a": sourceID, "unit-b": sourceID},
			wantPulls:   nil,
			wantDeletes: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			source := &fileRequests{failFor: tc.delFailFor}
			sourceServer := httptest.NewServer(http.HandlerFunc(source.handler))
			t.Cleanup(sourceServer.Close)

			target := &fileRequests{failFor: tc.pullFailFor}
			targetServer := httptest.NewServer(http.HandlerFunc(target.handler))
			t.Cleanup(targetServer.Close)

			cluster := moverCluster{
				members: map[int64]cowsqldb.NodeInfo{
					sourceID: {Name: "member02", Address: "192.0.2.2:8443", Heartbeat: time.Now()},
					targetID: {Name: "member03", Address: "192.0.2.3:8443", Heartbeat: tc.targetBeat},
				},
				urls: map[string]string{
					"192.0.2.2:8443": sourceServer.URL,
					"192.0.2.3:8443": targetServer.URL,
				},
			}

			repo := &fileTargets{
				units: []memberfiles.Unit{
					{Store: "images", Name: "unit-a", ID: 1},
					{Store: "images", Name: "unit-b", ID: 2},
				},
				targets:   map[string]int64{"unit-a": sourceID, "unit-b": sourceID},
				setErrFor: tc.setErrFor,
			}

			mover := memberfiles.NewMover(func() memberfiles.Cluster { return cluster }, repo)

			// Run test
			err := mover.Move(t.Context(), "member02", tc.targetName)

			// Assert
			tc.wantErr(t, err, "the error of the move")
			require.Equal(t, tc.wantTargets, repo.targets, "the recorded member of each unit")
			require.Equal(t, tc.wantPulls, target.requests, "the units the target pulled from the source")
			require.Equal(t, tc.wantDeletes, source.requests, "the units deleted on the source")
		})
	}
}
