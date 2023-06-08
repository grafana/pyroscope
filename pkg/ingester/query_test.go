package ingester

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	pushv1 "github.com/grafana/phlare/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/objstore/client"
	"github.com/grafana/phlare/pkg/objstore/providers/filesystem"
	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	"github.com/grafana/phlare/pkg/phlaredb"
	"github.com/grafana/phlare/pkg/tenant"
)

// func Test_selectMerge(t *testing.T) {
// 	cfg := defaultIngesterTestConfig(t)
// 	profileStore, err := profilestore.New(log.NewNopLogger(), nil, trace.NewNoopTracerProvider(), defaultProfileStoreTestConfig(t))
// 	require.NoError(t, err)

// 	d, err := New(cfg, log.NewNopLogger(), nil, profileStore)
// 	require.NoError(t, err)
// 	resp, err := d.Push(context.Background(), connect.NewRequest(&pushv1.PushRequest{
// 		Series: []*pushv1.RawProfileSeries{
// 			{
// 				Labels: []*commonv1.LabelPair{
// 					{Name: "__name__", Value: "memory"},
// 				},
// 				Samples: []*pushv1.RawSample{
// 					{
// 						RawProfile: generateProfile(
// 							t, "inuse_space", "bytes", "space", "bytes", time.Now().Add(-1*time.Minute),
// 							[]int64{1, 1},
// 							[][]string{
// 								{"bar", "foo"},
// 								{"buzz", "foo"},
// 							},
// 						),
// 					},
// 				},
// 			},
// 		},
// 	}))

// 	require.NoError(t, err)
// 	require.NotNil(t, resp)
// 	f, err := d.selectMerge(context.Background(), profileQuery{
// 		name:       "memory",
// 		sampleType: "inuse_space",
// 		sampleUnit: "bytes",
// 		periodType: "space",
// 		periodUnit: "bytes",
// 	}, 0, int64(model.Latest))
// 	require.NoError(t, err)

// 	// aggregate plan have no guarantee of order so we sort the results
// 	sort.Strings(f.Flamebearer.Names)

// 	require.Equal(t, []string{"bar", "buzz", "foo", "total"}, f.Flamebearer.Names)
// 	require.Equal(t, flamebearer.FlamebearerMetadataV1{
// 		Format:     "single",
// 		Units:      "bytes",
// 		Name:       "inuse_space",
// 		SampleRate: 100,
// 	}, f.Metadata)
// 	require.Equal(t, 2, f.Flamebearer.NumTicks)
// 	require.Equal(t, 1, f.Flamebearer.MaxSelf)
// 	require.Equal(t, []int{0, 2, 0, 0}, f.Flamebearer.Levels[0])
// 	require.Equal(t, []int{0, 2, 0, 1}, f.Flamebearer.Levels[1])
// 	require.Equal(t, []int{0, 1, 1}, f.Flamebearer.Levels[2][:3])
// 	require.Equal(t, []int{0, 1, 1}, f.Flamebearer.Levels[2][4:7])
// 	require.True(t, f.Flamebearer.Levels[2][3] == 3 || f.Flamebearer.Levels[2][3] == 2)
// 	require.True(t, f.Flamebearer.Levels[2][7] == 3 || f.Flamebearer.Levels[2][7] == 2)
// 	require.NoError(
// 		t,
// 		profileStore.Close(),
// 	)
// }

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
	}, fs, &fakeLimits{})
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

/*
func Test_selectProfiles(t *testing.T) {
	cfg := defaultIngesterTestConfig(t)
	logger := log.NewLogfmtLogger(os.Stdout)
	storeCfg := defaultProfileStoreTestConfig(t)
	profileStore, err := profilestore.New(logger, nil, trace.NewNoopTracerProvider(), storeCfg)
	require.NoError(t, err)

	d, err := New(cfg, log.NewLogfmtLogger(os.Stdout), nil, profileStore)
	require.NoError(t, err)

	resp, err := d.Push(context.Background(), connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*commonv1.LabelPair{
					{Name: "__name__", Value: "memory"},
					{Name: "cluster", Value: "us-central1"},
					{Name: "foo", Value: "bar"},
				},
				Samples: []*pushv1.RawSample{
					{
						RawProfile: generateProfile(
							t, "inuse_space", "bytes", "space", "bytes", time.Unix(1, 0),
							[]int64{1, 2},
							[][]string{
								{"foo", "bar", "buzz"},
								{"buzz", "baz", "foo"},
							},
						),
					},
				},
			},
			{
				Labels: []*commonv1.LabelPair{
					{Name: "__name__", Value: "memory"},
					{Name: "cluster", Value: "us-east1"},
				},
				Samples: []*pushv1.RawSample{
					{
						RawProfile: generateProfile(
							t, "inuse_space", "bytes", "space", "bytes", time.Unix(2, 0),
							[]int64{4, 5, 6},
							[][]string{
								{"foo", "bar", "buzz"},
								{"buzz", "baz", "foo"},
								{"1", "2", "3"},
							},
						),
					},
				},
			},
		},
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)

	res, err := d.SelectProfiles(context.Background(), connect.NewRequest(&ingestv1.SelectProfilesRequest{
		LabelSelector: `{cluster=~".*"}`,
		Type: &ingestv1.ProfileType{
			Name:       "memory",
			SampleType: "inuse_space",
			SampleUnit: "bytes",
			PeriodType: "space",
			PeriodUnit: "bytes",
		},
		Start: 0,
		End:   int64(model.Latest),
	}))
	require.NoError(t, err)
	sort.Slice(res.Msg.Profiles, func(i, j int) bool {
		return res.Msg.Profiles[i].Timestamp < res.Msg.Profiles[j].Timestamp
	})
	require.Equal(t, 2, len(res.Msg.Profiles))
	require.Equal(t, 2, len(res.Msg.Profiles[0].Labels))
	require.Equal(t, 1, len(res.Msg.Profiles[1].Labels))

	require.Equal(t, "cluster", res.Msg.Profiles[0].Labels[0].Name)
	require.Equal(t, "us-central1", res.Msg.Profiles[0].Labels[0].Value)
	require.Equal(t, "foo", res.Msg.Profiles[0].Labels[1].Name)
	require.Equal(t, "bar", res.Msg.Profiles[0].Labels[1].Value)
	require.Equal(t, "cluster", res.Msg.Profiles[1].Labels[0].Name)
	require.Equal(t, "us-east1", res.Msg.Profiles[1].Labels[0].Value)

	require.Equal(t, 2, len(res.Msg.Profiles[0].Stacktraces))
	require.Equal(t, 3, len(res.Msg.Profiles[1].Stacktraces))

	stackTracesID := [][]byte{}
	for _, p := range res.Msg.Profiles {
		for _, s := range p.Stacktraces {
			stackTracesID = append(stackTracesID, s.ID)
		}
	}

	symbolsReponse, err := d.SymbolizeStacktraces(context.Background(), connect.NewRequest(&ingestv1.SymbolizeStacktraceRequest{Ids: stackTracesID}))
	require.NoError(t, err)

	var stacktraces []string
	for _, p := range symbolsReponse.Msg.Locations {
		stracktrace := strings.Builder{}
		for j, l := range p.Ids {
			if j > 0 {
				stracktrace.WriteString("|")
			}
			stracktrace.WriteString(symbolsReponse.Msg.FunctionNames[l])

		}
		stacktraces = append(stacktraces, stracktrace.String())

	}
	sort.Strings(stacktraces)
	require.Equal(t, []string{"1|2|3", "buzz|baz|foo", "buzz|baz|foo", "foo|bar|buzz", "foo|bar|buzz"}, stacktraces)
	require.Equal(t, 5, len(symbolsReponse.Msg.Locations))
}

func generateProfile(
	t *testing.T,
	sampleType, sampleUnit, periodType, periodUnit string,
	ts time.Time,
	values []int64,
	locations [][]string,
) []byte {
	t.Helper()
	buf := bytes.NewBuffer(nil)
	mapping := &profile.Mapping{
		ID: 1,
	}
	functionMap := map[string]uint64{}
	locMap := map[string]*profile.Location{}
	fns := []*profile.Function{}
	locs := []*profile.Location{}
	id := uint64(1)
	for _, location := range locations {
		for _, function := range location {
			if _, ok := functionMap[function]; !ok {
				functionMap[function] = id
				fn := &profile.Function{
					ID:        id,
					Name:      function,
					StartLine: 1,
				}
				fns = append(fns, fn)
				loc := &profile.Location{
					ID:      id,
					Address: 0,
					Mapping: mapping,
					Line: []profile.Line{
						{Function: fn, Line: 1},
					},
				}
				locMap[function] = loc
				locs = append(locs, loc)
				id++
			}
		}
	}
	var samples []*profile.Sample
	for i, loc := range locations {
		s := &profile.Sample{
			Value: []int64{values[i]},
		}
		samples = append(samples, s)
		for _, function := range loc {
			s.Location = append(s.Location, locMap[function])
		}
	}
	p := &profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: sampleType, Unit: sampleUnit},
		},
		PeriodType: &profile.ValueType{
			Type: periodType,
			Unit: periodUnit,
		},
		DurationNanos: 0,
		Period:        3,
		TimeNanos:     ts.UnixNano(),
		Sample:        samples,
		Mapping: []*profile.Mapping{
			mapping,
		},
		Function: fns,
		Location: locs,
	}
	require.NoError(t, p.Write(buf))
	return buf.Bytes()
}
*/
