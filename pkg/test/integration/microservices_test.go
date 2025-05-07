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
	"github.com/grafana/pyroscope/pkg/tenant"
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
		now: time.Now().Truncate(time.Second),
		perTenantData: map[string]tenantParams{
			"tenant-a": {
				serviceCount: 100,
				samples:      5,
			},
			"tenant-b": {
				serviceCount: 1,
				samples:      1,
			},
			"tenant-not-existing": {},
		},
		querier: x.QueryClient(),
		pusher:  x.PushClient(),
	}
}

type tenantParams struct {
	serviceCount int
	samples      int
}

type testCtx struct {
	now time.Time

	perTenantData map[string]tenantParams
	querier       querierv1connect.QuerierServiceClient
	pusher        pushv1connect.PusherServiceClient
}

func (tc *testCtx) pushProfiles(ctx context.Context, t *testing.T) {
	g, gctx := errgroup.WithContext(ctx)

	g.SetLimit(20)
	for tenantID, params := range tc.perTenantData {
		gctx := tenant.InjectTenantID(gctx, tenantID)
		// for loop range over serviceCount
		for i := 0; i < params.serviceCount; i++ {
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
				for j := 0; j < params.samples; j++ {
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
	}
	require.NoError(t, g.Wait())

}

func (tc *testCtx) runQueryTest(ctx context.Context, t *testing.T) {
	t.Run("QuerySeries", func(t *testing.T) {
		for tenantID, params := range tc.perTenantData {
			t.Run(tenantID, func(t *testing.T) {
				ctx := tenant.InjectTenantID(ctx, tenantID)
				resp, err := tc.querier.Series(ctx, connect.NewRequest(&querierv1.SeriesRequest{
					Start:      tc.now.Add(-time.Hour).UnixMilli(),
					End:        tc.now.Add(time.Hour).UnixMilli(),
					LabelNames: []string{"__profile_type__", "service_name"},
				}))
				require.NoError(t, err)
				require.Len(t, resp.Msg.LabelsSet, params.serviceCount)
			})
		}
	})
	t.Run("QueryLabelNames", func(t *testing.T) {
		for tenantID, params := range tc.perTenantData {
			t.Run(tenantID, func(t *testing.T) {
				ctx := tenant.InjectTenantID(ctx, tenantID)
				resp, err := tc.querier.LabelNames(ctx, connect.NewRequest(&typesv1.LabelNamesRequest{
					Start: tc.now.Add(-time.Hour).UnixMilli(),
					End:   tc.now.Add(time.Hour).UnixMilli(),
				}))
				require.NoError(t, err)

				// no services, no label names
				if params.serviceCount == 0 {
					assert.Len(t, resp.Msg.Names, 0)
					return
				}

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
		}
	})

	t.Run("QueryLabelValues", func(t *testing.T) {
		for tenantID, params := range tc.perTenantData {
			t.Run(tenantID, func(t *testing.T) {
				ctx := tenant.InjectTenantID(ctx, tenantID)
				resp, err := tc.querier.LabelValues(ctx, connect.NewRequest(&typesv1.LabelValuesRequest{
					Start: tc.now.Add(-time.Hour).UnixMilli(),
					End:   tc.now.Add(time.Hour).UnixMilli(),
					Name:  "service_name",
				}))
				require.NoError(t, err)

				// no services, no label values
				if params.serviceCount == 0 {
					assert.Len(t, resp.Msg.Names, 0)
					return
				}

				// loop over numbers from i too serviceCount
				expectedValues := make([]string, params.serviceCount)
				for i := 0; i < params.serviceCount; i++ {
					// check if the service name is in the response
					expectedValues[i] = fmt.Sprintf("test-service-%d", i)
				}
				sort.Strings(expectedValues)
				assert.Equal(t, expectedValues, resp.Msg.Names)
			})
		}
	})

	t.Run("QuerySelectMergeProfile", func(t *testing.T) {
		for tenantID, params := range tc.perTenantData {
			t.Run(tenantID, func(t *testing.T) {
				ctx := tenant.InjectTenantID(ctx, tenantID)
				req := &querierv1.SelectMergeProfileRequest{
					ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
					LabelSelector: "{}",
					Start:         tc.now.Add(-time.Hour).UnixMilli(),
					End:           tc.now.Add(time.Hour).UnixMilli(),
				}
				resp, err := tc.querier.SelectMergeProfile(ctx, connect.NewRequest(req))
				require.NoError(t, err)

				assert.Equal(t, req.End*1e6, resp.Msg.TimeNanos)
				assert.Equal(t, int64(7200000000000), resp.Msg.DurationNanos)

				// no services, no samples profile
				if params.serviceCount == 0 {
					return
				}

				assert.Equal(t,
					[]*profilev1.ValueType{
						{Type: 6, Unit: 5},
					}, resp.Msg.SampleType,
				)

				// boz samples
				bozSamples := 0
				for i := 0; i < params.serviceCount; i++ {
					for j := 0; j < params.samples; j++ {
						if (i+j)%3 == 0 {
							bozSamples += 3
						}
					}
				}

				assert.Equal(t,
					[]*profilev1.Sample{
						{LocationId: []uint64{1, 2, 3}, Value: []int64{int64(params.serviceCount)}},
						{LocationId: []uint64{1, 2, 4}, Value: []int64{int64(bozSamples)}},
					}, resp.Msg.Sample)
				assert.Equal(t,
					[]*profilev1.Mapping{
						{Id: 1, HasFunctions: true},
					}, resp.Msg.Mapping,
				)
				assert.Equal(t,
					[]*profilev1.Location{
						{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1}}},
						{Id: 2, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 2}}},
						{Id: 3, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 3}}},
						{Id: 4, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 4}}},
					}, resp.Msg.Location,
				)
				assert.Equal(t,
					[]*profilev1.Function{
						{Id: 1, Name: 1},
						{Id: 2, Name: 2},
						{Id: 3, Name: 3},
						{Id: 4, Name: 4},
					}, resp.Msg.Function,
				)
				assert.Equal(t,
					[]string{"", "foo", "bar", "baz", "boz", "nanoseconds", "cpu"},
					resp.Msg.StringTable,
				)
				assert.Equal(t,
					&profilev1.ValueType{Type: 6, Unit: 5},
					resp.Msg.PeriodType,
				)
			})
		}
	})
}
