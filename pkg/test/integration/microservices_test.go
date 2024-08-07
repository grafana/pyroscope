package integration

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
	"github.com/grafana/pyroscope/pkg/test/integration/cluster"
)

// TestMicroServicesIntegration tests the integration of the microservices in a
// similar to is actually run in the scalable/high availability setup.
//
// After the cluster is fully started, it pushes profiles for a number of services
// and then queries the series, label names and label values. It then stops some
// of the services and runs the same queries again to check if the cluster is still
// able to respond to queries.
func TestMicroServicesIntegration(t *testing.T) {
	c := cluster.NewMicroServiceCluster()
	ctx := context.Background()

	require.NoError(t, c.Prepare())
	for _, comp := range c.Components {
		t.Log(comp.String())
	}

	// start returns as soon the cluster is ready
	require.NoError(t, c.Start(ctx))
	t.Log("Cluster ready")
	defer func() {
		waitStopped := c.Stop()
		require.NoError(t, waitStopped(ctx))
	}()

	tc := newTestCtx(c)
	t.Run("PushProfiles", func(t *testing.T) {
		tc.pushProfiles(ctx, t)
	})

	t.Run("HealthyCluster", func(t *testing.T) {
		tc.runQueryTest(ctx, t)
	})

	componentsToStop := map[string]struct{}{"store-gateway": {}, "ingester": {}}
	g, gctx := errgroup.WithContext(ctx)
	for _, comp := range c.Components {
		if _, ok := componentsToStop[comp.Target]; ok {
			t.Logf("Stopping %s", comp.Target)
			awaitStop := comp.Stop()
			delete(componentsToStop, comp.Target)
			g.Go(func() error {
				return awaitStop(gctx)
			})
		}
	}
	// wait for services being stopped
	require.NoError(t, g.Wait())

	t.Run("DegradedCluster", func(t *testing.T) {
		tc.runQueryTest(ctx, t)
	})

}

func newTestCtx(x interface {
	PushClient() pushv1connect.PusherServiceClient
	QueryClient() querierv1connect.QuerierServiceClient
}) *testCtx {
	return &testCtx{
		now:          time.Now().Truncate(time.Second),
		serviceCount: 100,
		samples:      5,
		querier:      x.QueryClient(),
		pusher:       x.PushClient(),
	}
}

type testCtx struct {
	now          time.Time
	serviceCount int
	samples      int

	querier querierv1connect.QuerierServiceClient
	pusher  pushv1connect.PusherServiceClient
}

func (tc *testCtx) pushProfiles(ctx context.Context, t *testing.T) {
	g, gctx := errgroup.WithContext(ctx)

	g.SetLimit(20)
	// for loop range over serviceCount
	for i := 0; i < tc.serviceCount; i++ {
		var i = i
		g.Go(func() error {
			serviceName := fmt.Sprintf("test-service-%d", i)
			builder := testhelper.NewProfileBuilder(int64(1)).
				CPUProfile().
				WithLabels(
					"job", "test",
					"service_name", serviceName,
				)
			builder.ForStacktraceString("foo", "bar", "baz").AddSamples(1)
			for j := 0; j < tc.samples; j++ {
				builder.TimeNanos = tc.now.Add(time.Duration(j) * 5 * time.Second).UnixNano()
				if (i+j)%3 == 0 {
					builder.ForStacktraceString("foo", "bar", "boz").AddSamples(3)
				}
			}

			rawProfile, err := builder.MarshalVT()
			require.NoError(t, err)

			_, err = tc.pusher.Push(gctx, connect.NewRequest(&pushv1.PushRequest{
				Series: []*pushv1.RawProfileSeries{{
					Labels:  builder.Labels,
					Samples: []*pushv1.RawSample{{RawProfile: rawProfile}},
				}},
			}))
			return err
		})
	}
	require.NoError(t, g.Wait())

}

func (tc *testCtx) runQueryTest(ctx context.Context, t *testing.T) {
	t.Run("QuerySeries", func(t *testing.T) {
		resp, err := tc.querier.Series(ctx, connect.NewRequest(&querierv1.SeriesRequest{
			Start:      tc.now.Add(-time.Hour).UnixMilli(),
			End:        tc.now.Add(time.Hour).UnixMilli(),
			LabelNames: []string{"__profile_type__", "job", "service_name"},
		}))
		require.NoError(t, err)
		require.Len(t, resp.Msg.LabelsSet, 100)
	})
	t.Run("QueryLabelNames", func(t *testing.T) {
		resp, err := tc.querier.LabelNames(ctx, connect.NewRequest(&typesv1.LabelNamesRequest{
			Start: tc.now.Add(-time.Hour).UnixMilli(),
			End:   tc.now.Add(time.Hour).UnixMilli(),
		}))
		require.NoError(t, err)
		assert.Equal(t, []string{
			"__name__",
			"__period_type__",
			"__period_unit__",
			"__profile_type__",
			"__service_name__",
			"__type__",
			"__unit__",
			"job",
			"service_name",
		}, resp.Msg.Names)
	})

	t.Run("QueryLabelValues", func(t *testing.T) {
		resp, err := tc.querier.LabelValues(ctx, connect.NewRequest(&typesv1.LabelValuesRequest{
			Start: tc.now.Add(-time.Hour).UnixMilli(),
			End:   tc.now.Add(time.Hour).UnixMilli(),
			Name:  "service_name",
		}))
		require.NoError(t, err)
		// loop over numbers from i too serviceCount
		expectedValues := make([]string, tc.serviceCount)
		for i := 0; i < tc.serviceCount; i++ {
			// check if the service name is in the response
			expectedValues[i] = fmt.Sprintf("test-service-%d", i)
		}
		sort.Strings(expectedValues)
		assert.Equal(t, expectedValues, resp.Msg.Names)
	})

	t.Run("QuerySelectMergeProfile", func(t *testing.T) {
		req := &querierv1.SelectMergeProfileRequest{
			ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
			LabelSelector: "{}",
			Start:         tc.now.Add(-time.Hour).UnixMilli(),
			End:           tc.now.Add(time.Hour).UnixMilli(),
		}
		resp, err := tc.querier.SelectMergeProfile(ctx, connect.NewRequest(req))
		require.NoError(t, err)

		expected := &profilev1.Profile{
			SampleType: []*profilev1.ValueType{
				{Type: 6, Unit: 5},
			},
			Sample: []*profilev1.Sample{
				{LocationId: []uint64{1, 2, 3}, Value: []int64{100}},
				{LocationId: []uint64{1, 2, 4}, Value: []int64{501}},
			},
			Mapping: []*profilev1.Mapping{{Id: 1, HasFunctions: true}},
			Location: []*profilev1.Location{
				{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1}}},
				{Id: 2, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 2}}},
				{Id: 3, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 3}}},
				{Id: 4, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 4}}},
			},
			Function: []*profilev1.Function{
				{Id: 1, Name: 1},
				{Id: 2, Name: 2},
				{Id: 3, Name: 3},
				{Id: 4, Name: 4},
			},
			StringTable:       []string{"", "foo", "bar", "baz", "boz", "nanoseconds", "cpu"},
			TimeNanos:         req.End * 1e6,
			DurationNanos:     7200000000000,
			PeriodType:        &profilev1.ValueType{Type: 6, Unit: 5},
			Period:            1000000000,
			DefaultSampleType: 6,
		}
		require.Equal(t, expected.String(), resp.Msg.String())
	})
}
