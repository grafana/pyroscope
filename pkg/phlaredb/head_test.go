package phlaredb

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	phlarecontext "github.com/grafana/pyroscope/pkg/phlare/context"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/pprof"
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

func TestHead_SelectMatchingProfiles_Order(t *testing.T) {
	ctx := testContext(t)
	const n = 15
	head, err := NewHead(ctx, Config{
		DataPath: t.TempDir(),
		Parquet: &ParquetConfig{
			MaxBufferRowCount: n - 1,
		},
	}, NoLimit)
	require.NoError(t, err)

	c := make(chan struct{})
	var closeOnce sync.Once
	head.profiles.onFlush = func() {
		closeOnce.Do(func() {
			close(c)
		})
	}

	now := time.Now()
	for i := 0; i < n; i++ {
		x := newProfileFoo()
		// Make sure some of our profiles have matching timestamps.
		x.TimeNanos = now.Add(time.Second * time.Duration(i-i%2)).UnixNano()
		require.NoError(t, head.Ingest(ctx, x, uuid.UUID{}, []*typesv1.LabelPair{
			{Name: "job", Value: "foo"},
			{Name: "x", Value: strconv.Itoa(i)},
		}...))
	}

	<-c
	q := head.Queriers()
	assert.Equal(t, 2, len(q)) // on-disk and in-memory parts.

	typ, err := phlaremodel.ParseProfileTypeSelector(":type:unit:type:unit")
	require.NoError(t, err)
	req := &ingestv1.SelectProfilesRequest{
		LabelSelector: "{}",
		Type:          typ,
		End:           now.Add(time.Hour).UnixMilli(),
	}

	profiles := make([]Profile, 0, n)
	for _, b := range q {
		i, err := b.SelectMatchingProfiles(ctx, req)
		require.NoError(t, err)
		s, err := iter.Slice(i)
		require.NoError(t, err)
		profiles = append(profiles, s...)
	}

	assert.Equal(t, n, len(profiles))
	for i, p := range profiles {
		x, err := strconv.Atoi(p.Labels().Get("x"))
		require.NoError(t, err)
		require.Equal(t, i, x, "SelectMatchingProfiles order mismatch")
	}
}

func TestHeadFlush(t *testing.T) {
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
		profile := parseProfile(t, profilePaths[pos])
		require.NoError(t, head.Ingest(ctx, profile, uuid.New()))
	}

	require.NoError(t, head.Flush(ctx))
	require.NoError(t, head.Move())

	b, err := filesystem.NewBucket(filepath.Dir(head.localPath))
	require.NoError(t, err)
	q := NewBlockQuerier(ctx, b)
	metas, err := q.BlockMetas(ctx)
	require.NoError(t, err)

	expectedMeta := []*block.Meta{
		{
			ULID:    head.meta.ULID,
			MinTime: head.meta.MinTime,
			MaxTime: head.meta.MaxTime,
			Stats: block.BlockStats{
				NumSamples:  14192,
				NumSeries:   8,
				NumProfiles: 11,
			},
			Labels: map[string]string{},
			Files: []block.File{
				{
					RelPath:   "index.tsdb",
					SizeBytes: 2484,
					TSDB: &block.TSDBFile{
						NumSeries: 8,
					},
				},
				{
					RelPath: "profiles.parquet",
					Parquet: &block.ParquetFile{
						NumRowGroups: 1,
						NumRows:      11,
					},
				},
				{
					RelPath: "symbols/functions.parquet",
					Parquet: &block.ParquetFile{
						NumRowGroups: 2,
						NumRows:      1423,
					},
				},
				{
					RelPath:   "symbols/index.symdb",
					SizeBytes: 308,
				},
				{
					RelPath: "symbols/locations.parquet",
					Parquet: &block.ParquetFile{
						NumRowGroups: 2,
						NumRows:      2469,
					},
				},
				{
					RelPath: "symbols/mappings.parquet",
					Parquet: &block.ParquetFile{
						NumRowGroups: 2,
						NumRows:      3,
					},
				},
				{
					RelPath:   "symbols/stacktraces.symdb",
					SizeBytes: 60366,
				},
				{
					RelPath: "symbols/strings.parquet",
					Parquet: &block.ParquetFile{
						NumRowGroups: 2,
						NumRows:      1722,
					},
				},
			},
			Compaction: block.BlockMetaCompaction{
				Level: 1,
				Sources: []ulid.ULID{
					head.meta.ULID,
				},
			},
			Version: 3,
		},
	}

	// Parquet files are not deterministic, their size can change for the same input so we don't check them.
	for i := range metas {
		for j := range metas[i].Files {
			if metas[i].Files[j].Parquet != nil && metas[i].Files[j].Parquet.NumRows != 0 {
				require.NotEmpty(t, metas[i].Files[j].SizeBytes)
				metas[i].Files[j].SizeBytes = 0
			}
		}
	}

	require.Equal(t, expectedMeta, metas)
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

func TestIsStale(t *testing.T) {
	head := newTestHead(t)
	now := time.Unix(0, time.Minute.Nanoseconds())

	// should not be stale if have not past the stale grace period
	head.updatedAt.Store(time.Unix(0, 0))
	require.False(t, head.isStale(now.UnixNano(), now))
	// should be stale as we have passed the stale grace period
	require.True(t, head.isStale(now.UnixNano(), now.Add(2*StaleGracePeriod)))
	// Should not be stale if maxT is not passed.
	require.False(t, head.isStale(now.Add(2*StaleGracePeriod).UnixNano(), now.Add(2*StaleGracePeriod)))
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
