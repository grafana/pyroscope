package metastore

import (
	"context"
	"testing"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
)

func TestDeletionFilterSemantics(t *testing.T) {
	for _, tt := range []struct {
		name, regex, service string
		matches              bool
	}{
		{"full match", "svc-a|svc-b", "svc-a", true},
		{"not substring", "svc-a", "prefix-svc-a", false},
		{"empty is not wildcard", "", "svc-a", false},
		{"empty matches empty", "", "", true},
		{"explicit wildcard", ".*", "svc-a", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			re, err := validateDeletionRequest(&metastorev1.DeleteDatasetsRequest{Tenant: "tenant-a", Filter: &metastorev1.DeleteFilter{ServiceNameRegex: tt.regex}})
			require.NoError(t, err)
			require.Equal(t, tt.matches, re.MatchString(tt.service))
		})
	}

	// Expressions that only become valid once wrapped in anchors would match
	// with different, non-anchored semantics; they must be rejected outright.
	for _, regex := range []string{"a)|(b", "svc-a)", "(svc-a", "["} {
		t.Run("rejects "+regex, func(t *testing.T) {
			_, err := validateDeletionRequest(&metastorev1.DeleteDatasetsRequest{Tenant: "tenant-a", Filter: &metastorev1.DeleteFilter{ServiceNameRegex: regex}})
			require.Equal(t, codes.InvalidArgument, status.Code(deletionRPCError(err)))
		})
	}
}

func TestPrepareDatasetDeletionRejection(t *testing.T) {
	// Invalid requests are successful FSM outcomes, even without a database.
	h := new(CompactionCommandHandler)
	response, err := h.PrepareDatasetDeletion(context.Background(), nil, &raft.Log{Term: 7}, &metastorev1.DeleteDatasetsRequest{})
	require.NoError(t, err)
	require.EqualValues(t, 7, response.Term)
	require.Nil(t, response.GetDeletion())
	require.Nil(t, response.PlanUpdate)
	require.Equal(t, raft_log.DatasetDeletionRejection_REASON_INVALID_REQUEST, response.GetRejection().Reason)
	raw, err := proto.Marshal(response)
	require.NoError(t, err)
	var decoded raft_log.PrepareDatasetDeletionResponse
	require.NoError(t, proto.Unmarshal(raw, &decoded))
	require.True(t, proto.Equal(response, &decoded))
	for _, tt := range []struct {
		reason raft_log.DatasetDeletionRejection_Reason
		code   codes.Code
	}{
		{raft_log.DatasetDeletionRejection_REASON_INVALID_REQUEST, codes.InvalidArgument},
		{raft_log.DatasetDeletionRejection_REASON_UNSUPPORTED, codes.Unimplemented},
		{raft_log.DatasetDeletionRejection_REASON_BLOCK_BUSY, codes.FailedPrecondition},
		{raft_log.DatasetDeletionRejection_REASON_SCHEDULER_FULL, codes.ResourceExhausted},
	} {
		err := deletionRejectionStatus(&raft_log.DatasetDeletionRejection{Reason: tt.reason, Message: "rejected"})
		require.Equal(t, tt.code, status.Code(err))
		require.Equal(t, "rejected", status.Convert(err).Message())
	}
}
