package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"
	"github.com/stretchr/testify/require"

	clusteringSvc "github.com/FuturFusion/operations-center/internal/cluster"
	membershipapi "github.com/FuturFusion/operations-center/internal/cluster/api"
	"github.com/FuturFusion/operations-center/shared/api"
)

// taskStatusMembership is a membership service only answering its task status and its members.
type taskStatusMembership struct {
	clusteringSvc.MembershipService

	status  membershipapi.InternalClusterTaskStatus
	members map[int64]cowsqldb.NodeInfo
}

func (m taskStatusMembership) TaskStatus() membershipapi.InternalClusterTaskStatus {
	return m.status
}

func (m taskStatusMembership) GetMembersByID(context.Context) (map[int64]cowsqldb.NodeInfo, error) {
	return m.members, nil
}

func TestMembershipHandler_taskStatusPost(t *testing.T) {
	t.Parallel()

	status := membershipapi.InternalClusterTaskStatus{
		Address: "192.0.2.1:8443",
		Tasks:   []membershipapi.InternalClusterTask{{Name: "cluster_update", State: membershipapi.TaskStopping}},
	}

	handler := &membershipHandler{service: taskStatusMembership{
		status:  status,
		members: map[int64]cowsqldb.NodeInfo{1: {ID: 1, Name: "m1", Address: "192.0.2.1:8443"}},
	}}

	tests := []struct {
		name   string
		target string

		wantCode   int
		wantStatus *membershipapi.InternalClusterTaskStatus
	}{
		{
			name:       "this member",
			wantCode:   http.StatusOK,
			wantStatus: &status,
		},
		{
			name:       "this member as the target",
			target:     "m1",
			wantCode:   http.StatusOK,
			wantStatus: &status,
		},
		{
			name:     "unknown target",
			target:   "m9",
			wantCode: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			target := "/internal/membership/tasks/:status"
			if tc.target != "" {
				target += "?target=" + tc.target
			}

			rec := httptest.NewRecorder()
			err := handler.taskStatusPost(httptest.NewRequest(http.MethodPost, target, nil)).Render(rec)
			require.NoError(t, err)
			require.Equal(t, tc.wantCode, rec.Code, rec.Body.String())

			if tc.wantStatus == nil {
				return
			}

			var resp api.Response
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

			var got membershipapi.InternalClusterTaskStatus
			require.NoError(t, resp.MetadataAsStruct(&got))
			require.Equal(t, *tc.wantStatus, got)
		})
	}
}
