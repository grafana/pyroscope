package integration

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
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
	"github.com/grafana/pyroscope/pkg/metastore/raftnode/raftnodepb"
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
func TestMicroServicesIntegrationV1(t *testing.T) {
	c := cluster.NewMicroServiceCluster()
	ctx := context.Background()

	require.NoError(t, c.Prepare(ctx))
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

func TestMicroServicesIntegrationV2(t *testing.T) {
	c := cluster.NewMicroServiceCluster(cluster.WithV2())
	ctx := context.Background()

	require.NoError(t, c.Prepare(ctx))
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

	// ingest some more data to compact the rest of the data we care about
	// TODO: This shouldn't be necessary see https://github.com/grafana/pyroscope/issues/4193.
	pushCtx, pushCancel := context.WithCancel(ctx)
	g, gctx := errgroup.WithContext(pushCtx)
	g.SetLimit(4)
	for i := 0; i < 200; i++ {
		g.Go(func() error {
			p, err := testhelper.NewProfileBuilder(tc.now.UnixNano()).
				CPUProfile().
				ForStacktraceString("foo", "bar", "baz").AddSamples(1).
				MarshalVT()
			require.NoError(t, err)

			pctx := tenant.InjectTenantID(gctx, fmt.Sprintf("dummy-tenant-%d", i))
			_, err = tc.pusher.Push(pctx, connect.NewRequest(&pushv1.PushRequest{
				Series: []*pushv1.RawProfileSeries{{
					Labels: []*typesv1.LabelPair{
						{Name: "service_name", Value: fmt.Sprintf("dummy-service/%d", i)},
						{Name: "__name__", Value: "process_cpu"},
					},
					Samples: []*pushv1.RawSample{{RawProfile: p}},
				}},
			}))
			return err
		})
	}
	defer func() {
		pushCancel()
		err := g.Wait()
		if !errors.Is(err, context.Canceled) {
			require.NoError(t, g.Wait())
		}
	}()

	// await compaction so tenant wide index is available
	require.Eventually(t, func() bool {
		jobs, err := c.CompactionJobsFinished(ctx)
		return err == nil && jobs > 0
	}, time.Minute, time.Second)
	t.Log("Compaction worker finished")

	// await until all tenants have all expected labelValues available
	// TODO: This shouldn't be necessary see https://github.com/grafana/pyroscope/issues/4193.
	require.Eventually(t, func() bool {
		for tenantID := range tc.perTenantData {
			ctx := tenant.InjectTenantID(ctx, tenantID)
			resp, err := tc.querier.LabelValues(ctx, connect.NewRequest(&typesv1.LabelValuesRequest{
				Start: tc.now.Add(-time.Hour).UnixMilli(),
				End:   tc.now.Add(time.Hour).UnixMilli(),
				Name:  "service_name",
			}))
			if err != nil {
				return false
			}
			if len(resp.Msg.Names) != tc.perTenantData[tenantID].serviceCount {
				return false
			}
		}
		return true
	}, time.Minute, time.Second)
	t.Log("All tenants have all expected labelValues available")

	tc.runQueryTest(ctx, t)

}

// TestMetastoreAutoJoin tests that a new metastore node can join an existing cluster
// using the auto-join feature without requiring bootstrap configuration.
func TestMetastoreAutoJoin(t *testing.T) {
	c := cluster.NewMicroServiceCluster(cluster.WithV2())
	ctx := context.Background()

	require.NoError(t, c.Prepare(ctx))
	for _, comp := range c.Components {
		t.Log(comp.String())
	}

	require.NoError(t, c.Start(ctx))
	defer func() {
		waitStopped := c.Stop()
		require.NoError(t, waitStopped(ctx))
	}()

	client, err := c.GetMetastoreRaftNodeClient()
	require.NoError(t, err)
	nodeInfo, err := client.NodeInfo(ctx, &raftnodepb.NodeInfoRequest{})
	require.NoError(t, err)
	require.Equal(t, 3, len(nodeInfo.Node.Peers), "initial cluster should have 3 peers")

	err = c.AddMetastoreWithAutoJoin(ctx)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		nodeInfo, err := client.NodeInfo(ctx, &raftnodepb.NodeInfoRequest{})
		if err != nil {
			t.Logf("Failed to get node info: %v", err)
			return false
		}
		t.Logf("Current peer count: %d", len(nodeInfo.Node.Peers))
		return len(nodeInfo.Node.Peers) == 4
	}, 30*time.Second, 1*time.Second, "new metastore should join cluster")
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
		for i := 0; i < params.serviceCount; i++ {
			var i = i
			g.Go(func() error {
				serviceName := fmt.Sprintf("%s/test-service-%d", tenantID, i)
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
	isV2 := strings.HasSuffix(t.Name(), "V2")
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

				// no services to check
				if params.serviceCount == 0 {
					return
				}

				expectedValues := make([]*typesv1.Labels, params.serviceCount)
				for i := 0; i < params.serviceCount; i++ {
					// check if the service name is in the response
					expectedValues[i] = &typesv1.Labels{
						Labels: []*typesv1.LabelPair{
							{
								Name:  "__profile_type__",
								Value: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
							},
							{
								Name:  "service_name",
								Value: fmt.Sprintf("%s/test-service-%d", tenantID, i),
							},
						},
					}
				}

				// sort the response by service name
				sort.Slice(resp.Msg.LabelsSet, func(i, j int) bool {
					return resp.Msg.LabelsSet[i].Labels[1].Value < resp.Msg.LabelsSet[j].Labels[1].Value
				})
				sort.Slice(expectedValues, func(i, j int) bool {
					return expectedValues[i].Labels[1].Value < expectedValues[j].Labels[1].Value
				})
				assert.Equal(t, expectedValues, resp.Msg.LabelsSet)
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

	validateProfileTypes := func(t *testing.T, serviceCount int, resp *querierv1.ProfileTypesResponse) {
		// no services, no label names
		if serviceCount == 0 {
			assert.Len(t, resp.ProfileTypes, 0)
			return
		}

		profileTypes := make([]string, 0, len(resp.ProfileTypes))
		for _, pt := range resp.ProfileTypes {
			profileTypes = append(profileTypes, pt.ID)
		}
		assert.Equal(t, []string{
			"process_cpu:cpu:nanoseconds:cpu:nanoseconds",
		}, profileTypes)
	}

	t.Run("QueryProfileTypesWithTimeRange", func(t *testing.T) {
		for tenantID, params := range tc.perTenantData {
			t.Run(tenantID, func(t *testing.T) {
				ctx := tenant.InjectTenantID(ctx, tenantID)

				// Query profile types with time range
				resp, err := tc.querier.ProfileTypes(ctx, connect.NewRequest(&querierv1.ProfileTypesRequest{
					Start: tc.now.Add(-time.Hour).UnixMilli(),
					End:   tc.now.Add(time.Hour).UnixMilli(),
				}))
				require.NoError(t, err)

				validateProfileTypes(t, params.serviceCount, resp.Msg)
			})
		}
	})

	// Note: Some ProfileTypes API clients rely on the ablility to call it without start/end.
	// See https://github.com/grafana/grafana/issues/110211
	t.Run("QueryProfileTypesWithoutTimeRange", func(t *testing.T) {
		for tenantID, params := range tc.perTenantData {
			t.Run(tenantID, func(t *testing.T) {
				ctx := tenant.InjectTenantID(ctx, tenantID)

				// Query profile types with time range
				resp, err := tc.querier.ProfileTypes(ctx, connect.NewRequest(&querierv1.ProfileTypesRequest{}))
				require.NoError(t, err)

				validateProfileTypes(t, params.serviceCount, resp.Msg)
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

				expectedValues := make([]string, params.serviceCount)
				for i := 0; i < params.serviceCount; i++ {
					// check if the service name is in the response
					expectedValues[i] = fmt.Sprintf("%s/test-service-%d", tenantID, i)
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

				// no services, no samples profile
				if params.serviceCount == 0 {
					return
				}

				// TODO: Experimental storage layer v2 doesn't support DurationNanos yet
				// https://github.com/grafana/pyroscope/issues/4192
				if !isV2 {
					assert.Equal(t, int64(7200000000000), resp.Msg.DurationNanos, "DurationNanos")
				}

				assert.Equal(t, req.End*1e6, resp.Msg.TimeNanos, "TimeNanos")

				assert.Equal(t,
					[]*profilev1.ValueType{
						{Type: 6, Unit: 5},
					}, resp.Msg.SampleType, "SampleType",
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
					}, resp.Msg.Sample, "Samples",
				)
				assert.Equal(t,
					[]*profilev1.Mapping{
						{Id: 1, HasFunctions: true},
					}, resp.Msg.Mapping, "Mappings",
				)
				assert.Equal(t,
					[]*profilev1.Location{
						{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1}}},
						{Id: 2, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 2}}},
						{Id: 3, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 3}}},
						{Id: 4, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 4}}},
					}, resp.Msg.Location, "Locations",
				)
				assert.Equal(t,
					[]*profilev1.Function{
						{Id: 1, Name: 1},
						{Id: 2, Name: 2},
						{Id: 3, Name: 3},
						{Id: 4, Name: 4},
					}, resp.Msg.Function, "Functions",
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
