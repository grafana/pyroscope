package query_frontend

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockmetastorev1"
)

func TestQueryFrontend_ProfileTypes(t *testing.T) {
	metaClient := mockmetastorev1.NewMockMetadataQueryServiceClient(t)
	limits := mockfrontend.NewMockLimits(t)
	f := NewQueryFrontend(log.NewNopLogger(), limits, metaClient, nil, nil)
	require.NotNil(t, f)

	limits.On("MaxQueryLookback", mock.Anything).Return(24 * time.Hour)
	limits.On("MaxQueryLength", mock.Anything).Return(2 * time.Hour)
	metaClient.On("QueryMetadata", mock.Anything, mock.Anything).Maybe().Return(&metastorev1.QueryMetadataResponse{
		Blocks: []*metastorev1.BlockMeta{
			{
				Datasets: []*metastorev1.Dataset{
					{ProfileTypes: []int32{1, 2, 3}},
					{ProfileTypes: []int32{4, 2}},
				},
				StringTable: []string{
					"",
					"memory:inuse_space:bytes:space:byte",
					"process_cpu:cpu:nanoseconds:cpu:nanoseconds",
					"mutex:delay:nanoseconds:mutex:count",
					"memory:alloc_in_new_tlab_objects:count:space:bytes",
				},
			},
			{
				Datasets: []*metastorev1.Dataset{
					{ProfileTypes: []int32{1, 2}},
				},
				StringTable: []string{
					"",
					"mutex:contentions:count:mutex:count",
					"mutex:delay:nanoseconds:mutex:count",
				},
			},
		},
	}, nil)

	ctx := tenant.InjectTenantID(context.Background(), "tenant")
	types, err := f.ProfileTypes(ctx, connect.NewRequest(&querierv1.ProfileTypesRequest{
		Start: time.Now().Add(-time.Hour).UnixMilli(),
		End:   time.Now().UnixMilli(),
	}))
	require.NoError(t, err)
	require.Equal(t, 5, len(types.Msg.ProfileTypes))
	require.Equal(t, "memory:alloc_in_new_tlab_objects:count:space:bytes", types.Msg.ProfileTypes[0].ID)
	require.Equal(t, "memory:inuse_space:bytes:space:byte", types.Msg.ProfileTypes[1].ID)
	require.Equal(t, "mutex:contentions:count:mutex:count", types.Msg.ProfileTypes[2].ID)
	require.Equal(t, "mutex:delay:nanoseconds:mutex:count", types.Msg.ProfileTypes[3].ID)
	require.Equal(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds", types.Msg.ProfileTypes[4].ID)
}
