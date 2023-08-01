package phlaredb

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/google/uuid"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	phlarecontext "github.com/grafana/pyroscope/pkg/phlare/context"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

type noLimit struct{}

func (n noLimit) AllowProfile(fp model.Fingerprint, lbs phlaremodel.Labels, tsNano int64) error {
	return nil
}

func (n noLimit) Stop() {}

var NoLimit = noLimit{}

func newTestHead(t testing.TB) *testHead {
	dataPath := t.TempDir()
	ctx := testContext(t)
	head, err := NewHead(ctx, Config{DataPath: dataPath}, NoLimit)
	require.NoError(t, err)
	return &testHead{Head: head, t: t, reg: phlarecontext.Registry(ctx).(*prometheus.Registry)}
}

type testHead struct {
	*Head
	t   testing.TB
	reg *prometheus.Registry
}

func (t *testHead) Flush(ctx context.Context) error {
	defer func() {
		t.t.Logf("flushing head of block %v", t.Head.meta.ULID)
	}()
	return t.Head.Flush(ctx)
}

func parseProfile(t testing.TB, path string) *profilev1.Profile {
	p, err := pprof.OpenFile(path)
	require.NoError(t, err, "failed opening profile: ", path)
	return p.Profile
}

var valueTypeStrings = []string{"unit", "type"}

func newValueType() *profilev1.ValueType {
	return &profilev1.ValueType{
		Unit: 1,
		Type: 2,
	}
}

func newProfileFoo() *profilev1.Profile {
	baseTable := append([]string{""}, valueTypeStrings...)
	baseTableLen := int64(len(baseTable)) + 0
	return &profilev1.Profile{
		Function: []*profilev1.Function{
			{
				Id:   1,
				Name: baseTableLen + 0,
			},
			{
				Id:   2,
				Name: baseTableLen + 1,
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Address:   0x1337,
			},
			{
				Id:        2,
				MappingId: 1,
				Address:   0x1338,
			},
		},
		Mapping: []*profilev1.Mapping{
			{Id: 1, Filename: baseTableLen + 2},
		},
		StringTable: append(baseTable, []string{
			"func_a",
			"func_b",
			"my-foo-binary",
		}...),
		TimeNanos:  123456,
		PeriodType: newValueType(),
		SampleType: []*profilev1.ValueType{newValueType()},
		Sample: []*profilev1.Sample{
			{
				Value:      []int64{0o123},
				LocationId: []uint64{1},
			},
			{
				Value:      []int64{1234},
				LocationId: []uint64{1, 2},
			},
		},
	}
}

func newProfileBar() *profilev1.Profile {
	baseTable := append([]string{""}, valueTypeStrings...)
	baseTableLen := int64(len(baseTable)) + 0
	return &profilev1.Profile{
		Function: []*profilev1.Function{
			{
				Id:   10,
				Name: baseTableLen + 1,
			},
			{
				Id:   21,
				Name: baseTableLen + 0,
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        113,
				MappingId: 1,
				Address:   0x1337,
				Line: []*profilev1.Line{
					{FunctionId: 10, Line: 1},
				},
			},
		},
		Mapping: []*profilev1.Mapping{
			{Id: 1, Filename: baseTableLen + 2},
		},
		StringTable: append(baseTable, []string{
			"func_b",
			"func_a",
			"my-bar-binary",
		}...),
		TimeNanos:  123456,
		PeriodType: newValueType(),
		SampleType: []*profilev1.ValueType{newValueType()},
		Sample: []*profilev1.Sample{
			{
				Value:      []int64{2345},
				LocationId: []uint64{113},
			},
		},
	}
}

func newProfileBaz() *profilev1.Profile {
	return &profilev1.Profile{
		Function: []*profilev1.Function{
			{
				Id:   25,
				Name: 1,
			},
		},
		StringTable: []string{
			"",
			"func_c",
		},
	}
}

func TestHeadMetrics(t *testing.T) {
	head := newTestHead(t)
	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), uuid.New()))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), uuid.New()))
	require.NoError(t, head.Ingest(context.Background(), newProfileBaz(), uuid.New()))
	time.Sleep(time.Second)
	require.NoError(t, testutil.GatherAndCompare(head.reg,
		strings.NewReader(`
# HELP pyroscope_head_ingested_sample_values_total Number of sample values ingested into the head per profile type.
# TYPE pyroscope_head_ingested_sample_values_total counter
pyroscope_head_ingested_sample_values_total{profile_name=""} 3
# HELP pyroscope_head_profiles_created_total Total number of profiles created in the head
# TYPE pyroscope_head_profiles_created_total counter
pyroscope_head_profiles_created_total{profile_name=""} 2
# HELP pyroscope_head_received_sample_values_total Number of sample values received into the head per profile type.
# TYPE pyroscope_head_received_sample_values_total counter
pyroscope_head_received_sample_values_total{profile_name=""} 3

# HELP pyroscope_head_size_bytes Size of a particular in memory store within the head phlaredb block.
# TYPE pyroscope_head_size_bytes gauge
pyroscope_head_size_bytes{type="functions"} 72
pyroscope_head_size_bytes{type="locations"} 152
pyroscope_head_size_bytes{type="mappings"} 96
pyroscope_head_size_bytes{type="profiles"} 388
pyroscope_head_size_bytes{type="stacktraces"} 0
pyroscope_head_size_bytes{type="strings"} 52

`),
		"pyroscope_head_received_sample_values_total",
		"pyroscope_head_profiles_created_total",
		"pyroscope_head_ingested_sample_values_total",
		"pyroscope_head_size_bytes",
	))
}

func TestHeadIngestFunctions(t *testing.T) {
	head := newTestHead(t)

	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), uuid.New()))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), uuid.New()))
	require.NoError(t, head.Ingest(context.Background(), newProfileBaz(), uuid.New()))

	require.Equal(t, 3, len(head.functions.slice))
	helper := &functionsHelper{}
	assert.Equal(t, functionsKey{Name: 3}, helper.key(head.functions.slice[0]))
	assert.Equal(t, functionsKey{Name: 4}, helper.key(head.functions.slice[1]))
	assert.Equal(t, functionsKey{Name: 7}, helper.key(head.functions.slice[2]))
}

func TestHeadIngestStrings(t *testing.T) {
	ctx := context.Background()
	head := newTestHead(t)

	r := &rewriter{}
	require.NoError(t, head.strings.ingest(ctx, newProfileFoo().StringTable, r))
	require.Equal(t, []string{"", "unit", "type", "func_a", "func_b", "my-foo-binary"}, head.strings.slice)
	require.Equal(t, stringConversionTable{0, 1, 2, 3, 4, 5}, r.strings)

	r = &rewriter{}
	require.NoError(t, head.strings.ingest(ctx, newProfileBar().StringTable, r))
	require.Equal(t, []string{"", "unit", "type", "func_a", "func_b", "my-foo-binary", "my-bar-binary"}, head.strings.slice)
	require.Equal(t, stringConversionTable{0, 1, 2, 4, 3, 6}, r.strings)

	r = &rewriter{}
	require.NoError(t, head.strings.ingest(ctx, newProfileBaz().StringTable, r))
	require.Equal(t, []string{"", "unit", "type", "func_a", "func_b", "my-foo-binary", "my-bar-binary", "func_c"}, head.strings.slice)
	require.Equal(t, stringConversionTable{0, 7}, r.strings)
}

func TestHeadIngestStacktraces(t *testing.T) {
	ctx := context.Background()
	head := newTestHead(t)

	require.NoError(t, head.Ingest(ctx, newProfileFoo(), uuid.MustParse("00000000-0000-0000-0000-00000000000a")))
	require.NoError(t, head.Ingest(ctx, newProfileBar(), uuid.MustParse("00000000-0000-0000-0000-00000000000b")))
	require.NoError(t, head.Ingest(ctx, newProfileBar(), uuid.MustParse("00000000-0000-0000-0000-00000000000c")))

	// expect 2 mappings
	require.Equal(t, 2, len(head.mappings.slice))
	assert.Equal(t, "my-foo-binary", head.strings.slice[head.mappings.slice[0].Filename])
	assert.Equal(t, "my-bar-binary", head.strings.slice[head.mappings.slice[1].Filename])

	// expect 3 profiles
	require.Equal(t, 3, len(head.profiles.slice))

	var samples []uint32
	for pos := range head.profiles.slice {
		samples = append(samples, head.profiles.slice[pos].Samples.StacktraceIDs...)
	}
	// expect 4 samples, 2 of which distinct: stacktrace ID is
	// only valid within the scope of the stacktrace partition,
	// which depends on the main binary mapping filename.
	require.Len(t, lo.Uniq(samples), 2)
	require.Len(t, samples, 4)
}

func TestHeadLabelValues(t *testing.T) {
	head := newTestHead(t)
	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), uuid.New(), &typesv1.LabelPair{Name: "job", Value: "foo"}, &typesv1.LabelPair{Name: "namespace", Value: "phlare"}))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), uuid.New(), &typesv1.LabelPair{Name: "job", Value: "bar"}, &typesv1.LabelPair{Name: "namespace", Value: "phlare"}))

	res, err := head.LabelValues(context.Background(), connect.NewRequest(&typesv1.LabelValuesRequest{Name: "cluster"}))
	require.NoError(t, err)
	require.Equal(t, []string{}, res.Msg.Names)

	res, err = head.LabelValues(context.Background(), connect.NewRequest(&typesv1.LabelValuesRequest{Name: "job"}))
	require.NoError(t, err)
	require.Equal(t, []string{"bar", "foo"}, res.Msg.Names)
}

func TestHeadLabelNames(t *testing.T) {
	head := newTestHead(t)
	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), uuid.New(), &typesv1.LabelPair{Name: "job", Value: "foo"}, &typesv1.LabelPair{Name: "namespace", Value: "phlare"}))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), uuid.New(), &typesv1.LabelPair{Name: "job", Value: "bar"}, &typesv1.LabelPair{Name: "namespace", Value: "phlare"}))

	res, err := head.LabelNames(context.Background(), connect.NewRequest(&typesv1.LabelNamesRequest{}))
	require.NoError(t, err)
	require.Equal(t, []string{"__period_type__", "__period_unit__", "__profile_type__", "__type__", "__unit__", "job", "namespace"}, res.Msg.Names)
}

func TestHeadSeries(t *testing.T) {
	head := newTestHead(t)
	fooLabels := phlaremodel.NewLabelsBuilder(nil).Set("namespace", "phlare").Set("job", "foo").Labels()
	barLabels := phlaremodel.NewLabelsBuilder(nil).Set("namespace", "phlare").Set("job", "bar").Labels()
	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), uuid.New(), fooLabels...))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), uuid.New(), barLabels...))

	lblBuilder := phlaremodel.NewLabelsBuilder(nil).
		Set("namespace", "phlare").
		Set("job", "foo").
		Set("__period_type__", "type").
		Set("__period_unit__", "unit").
		Set("__type__", "type").
		Set("__unit__", "unit").
		Set("__profile_type__", ":type:unit:type:unit")
	expected := lblBuilder.Labels()
	res, err := head.Series(context.Background(), connect.NewRequest(&ingestv1.SeriesRequest{Matchers: []string{`{job="foo"}`}}))
	require.NoError(t, err)
	require.Equal(t, []*typesv1.Labels{{Labels: expected}}, res.Msg.LabelsSet)

	// Test we can filter labelNames
	res, err = head.Series(context.Background(), connect.NewRequest(&ingestv1.SeriesRequest{LabelNames: []string{"job", "not-existing"}}))
	require.NoError(t, err)
	lblBuilder.Reset(nil)
	jobFoo := lblBuilder.Set("job", "foo").Labels()
	lblBuilder.Reset(nil)
	jobBar := lblBuilder.Set("job", "bar").Labels()
	require.Len(t, res.Msg.LabelsSet, 2)
	require.Contains(t, res.Msg.LabelsSet, &typesv1.Labels{Labels: jobFoo})
	require.Contains(t, res.Msg.LabelsSet, &typesv1.Labels{Labels: jobBar})
}

func TestHeadProfileTypes(t *testing.T) {
	head := newTestHead(t)
	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), uuid.New(), &typesv1.LabelPair{Name: "__name__", Value: "foo"}, &typesv1.LabelPair{Name: "job", Value: "foo"}, &typesv1.LabelPair{Name: "namespace", Value: "phlare"}))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), uuid.New(), &typesv1.LabelPair{Name: "__name__", Value: "bar"}, &typesv1.LabelPair{Name: "namespace", Value: "phlare"}))

	res, err := head.ProfileTypes(context.Background(), connect.NewRequest(&ingestv1.ProfileTypesRequest{}))
	require.NoError(t, err)
	require.Equal(t, []*typesv1.ProfileType{
		mustParseProfileSelector(t, "bar:type:unit:type:unit"),
		mustParseProfileSelector(t, "foo:type:unit:type:unit"),
	}, res.Msg.ProfileTypes)
}

func mustParseProfileSelector(t testing.TB, selector string) *typesv1.ProfileType {
	ps, err := phlaremodel.ParseProfileTypeSelector(selector)
	require.NoError(t, err)
	return ps
}

func TestHeadIngestRealProfiles(t *testing.T) {
	profilePaths := []string{
		"testdata/heap",
		"testdata/profile",
		"testdata/profile_uncompressed",
		"testdata/profile_python",
		"testdata/profile_java",
	}

	head := newTestHead(t)
	ctx := context.Background()

	for pos := range profilePaths {
		path := profilePaths[pos]
		t.Run(path, func(t *testing.T) {
			profile := parseProfile(t, profilePaths[pos])
			require.NoError(t, head.Ingest(ctx, profile, uuid.New()))
		})
	}

	require.NoError(t, head.Flush(ctx))
	t.Logf("strings=%d samples=%d", len(head.strings.slice), head.totalSamples.Load())
}

// TestHead_Concurrent_Ingest_Querying tests that the head can handle concurrent reads and writes.
func TestHead_Concurrent_Ingest_Querying(t *testing.T) {
	var (
		ctx = testContext(t)
		cfg = Config{
			DataPath: t.TempDir(),
		}
		head, err = NewHead(ctx, cfg, NoLimit)
	)
	require.NoError(t, err)

	// force different row group segements for profiles
	head.profiles.cfg = &ParquetConfig{MaxRowGroupBytes: 128000, MaxBufferRowCount: 10}

	wg := sync.WaitGroup{}

	profilesPerSeries := 33

	for i := 0; i < 3; i++ {
		wg.Add(1)
		// ingester
		go func(i int) {
			defer wg.Done()
			tick := time.NewTicker(time.Millisecond)
			defer tick.Stop()
			for j := 0; j < profilesPerSeries; j++ {
				<-tick.C
				require.NoError(t, ingestThreeProfileStreams(ctx, profilesPerSeries*i+j, head.Ingest))
			}
			t.Logf("ingest stream %s done", streams[i])
		}(i)

		// querier
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			tick := time.NewTicker(time.Millisecond)
			defer tick.Stop()

			tsToBeSeen := make(map[int64]struct{}, profilesPerSeries)
			for j := 0; j < profilesPerSeries; j++ {
				tsToBeSeen[int64(j*3+i)] = struct{}{}
			}

			for j := 0; j < 50; j++ {
				<-tick.C
				// now query the store
				params := &ingestv1.SelectProfilesRequest{
					Start:         0,
					End:           1000000000000,
					LabelSelector: fmt.Sprintf(`{stream="%s"}`, streams[i]),
					Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
				}

				queriers := head.Queriers()

				pIt, err := queriers.SelectMatchingProfiles(ctx, params)
				require.NoError(t, err)

				for pIt.Next() {
					ts := pIt.At().Timestamp().Unix()
					if (ts % 3) != int64(i) {
						panic("unexpected timestamp")
					}
					delete(tsToBeSeen, ts)
				}

				// finish once we have all the profiles
				if len(tsToBeSeen) == 0 {
					break
				}
			}
			t.Logf("read stream %s done", streams[i])
		}(i)

	}

	// TODO: We need to test if flushing misses out on ingested profiles

	wg.Wait()
}

func TestFlushMeta(t *testing.T) {
	b := newBlock(t, func() []*testhelper.ProfileBuilder {
		return []*testhelper.ProfileBuilder{
			testhelper.NewProfileBuilder(int64(time.Second*1)).
				CPUProfile().
				WithLabels(
					"job", "a",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
			testhelper.NewProfileBuilder(int64(time.Second*2)).
				CPUProfile().
				WithLabels(
					"job", "b",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
			testhelper.NewProfileBuilder(int64(time.Second*3)).
				CPUProfile().
				WithLabels(
					"job", "c",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
		}
	})

	require.Equal(t, []ulid.ULID{b.Meta().ULID}, b.Meta().Compaction.Sources)
	require.Equal(t, 1, b.Meta().Compaction.Level)
	require.Equal(t, false, b.Meta().Compaction.Deletable)
	require.Equal(t, false, b.Meta().Compaction.Failed)
	require.Equal(t, []string(nil), b.Meta().Compaction.Hints)
	require.Equal(t, []tsdb.BlockDesc(nil), b.Meta().Compaction.Parents)
	require.Equal(t, block.MetaVersion2, b.Meta().Version)
	require.Equal(t, model.Time(1000), b.Meta().MinTime)
	require.Equal(t, model.Time(3000), b.Meta().MaxTime)
	require.Equal(t, uint64(3), b.Meta().Stats.NumSeries)
	require.Equal(t, uint64(3), b.Meta().Stats.NumSamples)
	require.Equal(t, uint64(3), b.Meta().Stats.NumProfiles)
	require.Len(t, b.Meta().Files, 8)
	require.Equal(t, "functions.parquet", b.Meta().Files[0].RelPath)
	require.Equal(t, "index.tsdb", b.Meta().Files[1].RelPath)
	require.Equal(t, "locations.parquet", b.Meta().Files[2].RelPath)
	require.Equal(t, "mappings.parquet", b.Meta().Files[3].RelPath)
	require.Equal(t, "profiles.parquet", b.Meta().Files[4].RelPath)
	require.Equal(t, "strings.parquet", b.Meta().Files[5].RelPath)
	require.Equal(t, "symbols/index.symdb", b.Meta().Files[6].RelPath)
	require.Equal(t, "symbols/stacktraces.symdb", b.Meta().Files[7].RelPath)
}

func BenchmarkHeadIngestProfiles(t *testing.B) {
	var (
		profilePaths = []string{
			"testdata/heap",
			"testdata/profile",
		}
		profileCount = 0
	)

	head := newTestHead(t)
	ctx := context.Background()

	t.ReportAllocs()

	for n := 0; n < t.N; n++ {
		for pos := range profilePaths {
			p := parseProfile(t, profilePaths[pos])
			require.NoError(t, head.Ingest(ctx, p, uuid.New()))
			profileCount++
		}
	}
}
