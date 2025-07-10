package ingester

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	phlarecontext "github.com/grafana/pyroscope/pkg/pyroscope/context"
	"github.com/grafana/pyroscope/pkg/tenant"
)

func Test_QueryMetadata(t *testing.T) {
	dbPath := t.TempDir()
	logger := log.NewJSONLogger(os.Stdout)
	reg := prometheus.NewRegistry()
	ctx := phlarecontext.WithLogger(context.Background(), logger)
	ctx = phlarecontext.WithRegistry(ctx, reg)
	cfg := client.Config{
		StorageBackendConfig: client.StorageBackendConfig{
			Backend: client.Filesystem,
			Filesystem: filesystem.Config{
				Directory: dbPath,
			},
		},
	}

	fs, err := client.NewBucket(ctx, cfg, "storage")
	require.NoError(t, err)

	ing, err := New(ctx, defaultIngesterTestConfig(t), phlaredb.Config{
		DataPath:         dbPath,
		MaxBlockDuration: 30 * time.Hour,
	}, fs, &fakeLimits{}, 0)
	require.NoError(t, err)
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), ing))

	req := &connect.Request[pushv1.PushRequest]{
		Msg: &pushv1.PushRequest{
			Series: []*pushv1.RawProfileSeries{
				{
					Samples: []*pushv1.RawSample{
						{
							ID:         uuid.NewString(),
							RawProfile: testProfile(t),
						},
					},
				},
			},
		},
	}
	req.Msg.Series[0].Labels = phlaremodel.LabelsFromStrings("foo", "bar")
	_, err = ing.Push(tenant.InjectTenantID(context.Background(), "foo"), req)
	require.NoError(t, err)

	labelsValues, err := ing.LabelValues(tenant.InjectTenantID(context.Background(), "foo"), connect.NewRequest(&typesv1.LabelValuesRequest{Name: "foo"}))
	require.NoError(t, err)
	require.Equal(t, []string{"bar"}, labelsValues.Msg.Names)

	profileTypes, err := ing.ProfileTypes(tenant.InjectTenantID(context.Background(), "foo"), connect.NewRequest(&ingestv1.ProfileTypesRequest{}))
	require.NoError(t, err)
	expectedTypes := []string{
		":alloc_objects:count:space:bytes",
		":alloc_space:bytes:space:bytes",
		":inuse_objects:count:space:bytes",
		":inuse_space:bytes:space:bytes",
	}
	sort.Strings(expectedTypes)
	ids := make([]string, len(profileTypes.Msg.ProfileTypes))
	for i, t := range profileTypes.Msg.ProfileTypes {
		ids[i] = t.ID
	}
	sort.Strings(ids)
	require.Equal(t, expectedTypes, ids)
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), ing))
}
