//go:build integration

package querybackend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
	thanosobjstore "github.com/thanos-io/objstore"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/v2/pkg/segmentwriter/memdb"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

const syntheticTenant = "tenant-a"

type latencyBucket struct {
	thanosobjstore.Bucket
	ttfb      time.Duration
	bandwidth float64
	requests  atomic.Int64
}

func (b *latencyBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	if err := b.wait(ctx, 0); err != nil {
		return nil, err
	}
	return b.Bucket.Get(ctx, name)
}

func (b *latencyBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	if err := b.wait(ctx, length); err != nil {
		return nil, err
	}
	return b.Bucket.GetRange(ctx, name, off, length)
}

func (b *latencyBucket) wait(ctx context.Context, n int64) error {
	b.requests.Add(1)
	d := b.ttfb
	if n > 0 && b.bandwidth > 0 {
		d += time.Duration(float64(n) / b.bandwidth * float64(time.Second))
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

type syntheticBlock struct {
	services int
	// memory adds a memory profile for every CPU profile.
	memory    bool
	segments  int
	profiles  int
	samples   int
	stacks    int
	functions int
	maxDepth  int
}

func (c syntheticBlock) build(t testing.TB, bucket objstore.Bucket) *metastorev1.BlockMeta {
	rng := rand.New(rand.NewSource(42))
	stacks := make([][][]uint64, c.services)
	for s := range stacks {
		stacks[s] = c.stackTraces(rng)
	}
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	segments := make([]*metastorev1.BlockMeta, c.segments)
	for i := range segments {
		segments[i] = c.segment(t, rng, bucket, stacks, start.Add(time.Duration(i)*10*time.Second))
	}
	compacted, err := block.Compact(context.Background(), segments, bucket, block.WithCompactionTempDir(t.TempDir()))
	require.NoError(t, err)
	require.Len(t, compacted, 1)
	return compacted[0]
}

func (c syntheticBlock) stackTraces(rng *rand.Rand) [][]uint64 {
	stacks := make([][]uint64, c.stacks)
	for i := range stacks {
		depth := 1 + rng.Intn(c.maxDepth)
		stack := make([]uint64, depth)
		for d := range stack {
			n := min(c.functions, 4<<(depth-d-1))
			stack[d] = uint64(1 + rng.Intn(n))
		}
		stacks[i] = stack
	}
	return stacks
}

func (c syntheticBlock) segment(
	t testing.TB,
	rng *rand.Rand,
	bucket objstore.Bucket,
	stacks [][][]uint64,
	ts time.Time,
) *metastorev1.BlockMeta {
	ctx := context.Background()
	strings := metadata.NewStringTable()
	md := &metastorev1.BlockMeta{
		FormatVersion: 1,
		Id:            ulid.MustNew(ulid.Timestamp(ts), rng).String(),
		MinTime:       math.MaxInt64,
	}
	var buf bytes.Buffer
	for s := 0; s < c.services; s++ {
		service := fmt.Sprintf("service-%03d", s)
		head := memdb.NewHead(memdb.NewHeadMetricsWithPrefix(nil, ""))
		for p := 0; p < c.profiles; p++ {
			profileTime := ts.Add(time.Duration(p) * time.Second / time.Duration(c.profiles))
			head.Ingest(c.profile(rng, service, stacks[s], profileTime), uuid.New(), []*typesv1.LabelPair{
				{Name: phlaremodel.LabelNameServiceName, Value: service},
				{Name: "__name__", Value: "process_cpu"},
				{Name: "pod", Value: fmt.Sprintf("pod-%d", p%4)},
			}, nil)
			if c.memory {
				mp := c.profile(rng, service, stacks[s], profileTime)
				mp.StringTable = append(mp.StringTable, "alloc_space", "bytes", "space")
				n := int64(len(mp.StringTable))
				mp.SampleType = []*profilev1.ValueType{{Type: n - 3, Unit: n - 2}}
				mp.PeriodType = &profilev1.ValueType{Type: n - 1, Unit: n - 2}
				head.Ingest(mp, uuid.New(), []*typesv1.LabelPair{
					{Name: phlaremodel.LabelNameServiceName, Value: service},
					{Name: "__name__", Value: "memory"},
					{Name: "pod", Value: fmt.Sprintf("pod-%d", p%4)},
				}, nil)
			}
		}
		f, err := head.Flush(ctx)
		require.NoError(t, err)

		off := uint64(buf.Len())
		toc := []uint64{off, 0, 0}
		buf.Write(f.Profiles)
		toc[1] = uint64(buf.Len())
		buf.Write(f.Index)
		toc[2] = uint64(buf.Len())
		buf.Write(f.Symbols)

		lb := metadata.NewLabelBuilder(strings)
		for _, pt := range f.Meta.ProfileTypeNames {
			lb.WithLabelSet(phlaremodel.LabelNameServiceName, service, phlaremodel.LabelNameProfileType, pt)
		}
		ds := &metastorev1.Dataset{
			Tenant:          strings.Put(syntheticTenant),
			Name:            strings.Put(service),
			MinTime:         f.Meta.MinTimeNanos / 1e6,
			MaxTime:         f.Meta.MaxTimeNanos / 1e6,
			Size:            uint64(buf.Len()) - off,
			TableOfContents: toc,
			Labels:          lb.Build(),
		}
		md.MinTime = min(md.MinTime, ds.MinTime)
		md.MaxTime = max(md.MaxTime, ds.MaxTime)
		md.Datasets = append(md.Datasets, ds)
	}
	md.StringTable = strings.Strings
	md.MetadataOffset = uint64(buf.Len())
	require.NoError(t, metadata.Encode(&buf, md))
	md.Size = uint64(buf.Len())
	require.NoError(t, bucket.Upload(ctx, block.ObjectPath(md), &buf))
	return md
}

func (c syntheticBlock) profile(rng *rand.Rand, service string, stacks [][]uint64, ts time.Time) *profilev1.Profile {
	p := &profilev1.Profile{
		SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 2}},
		PeriodType:  &profilev1.ValueType{Type: 1, Unit: 2},
		Period:      10_000_000,
		TimeNanos:   ts.UnixNano(),
		StringTable: []string{"", "cpu", "nanoseconds"},
		Mapping:     []*profilev1.Mapping{{Id: 1, HasFunctions: true}},
	}
	for i := 1; i <= c.functions; i++ {
		p.StringTable = append(p.StringTable, fmt.Sprintf("%s/pkg%02d.func%05d", service, i%32, i))
		p.Function = append(p.Function, &profilev1.Function{Id: uint64(i), Name: int64(len(p.StringTable) - 1)})
		p.Location = append(p.Location, &profilev1.Location{
			Id:        uint64(i),
			MappingId: 1,
			Line:      []*profilev1.Line{{FunctionId: uint64(i)}},
		})
	}
	for i := 0; i < c.samples; i++ {
		stack := stacks[rng.Intn(1+rng.Intn(len(stacks)))]
		p.Sample = append(p.Sample, &profilev1.Sample{
			LocationId: slices.Clone(stack),
			Value:      []int64{int64(1+rng.Intn(10)) * p.Period},
		})
	}
	return p
}

func queryBlock(
	t testing.TB,
	reader *BlockReader,
	md *metastorev1.BlockMeta,
	query *queryv1.Query,
	datasetIndex bool,
) *queryv1.InvokeResponse {
	md = md.CloneVT()
	md.Datasets = slices.DeleteFunc(md.Datasets, func(ds *metastorev1.Dataset) bool {
		return (block.DatasetFormat(ds.Format) == block.DatasetFormat1) != datasetIndex
	})
	resp, err := reader.Invoke(context.Background(), &queryv1.InvokeRequest{
		StartTime:     md.MinTime,
		EndTime:       md.MaxTime + 1,
		LabelSelector: `{__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds"}`,
		QueryPlan:     queryplan.Build([]*metastorev1.BlockMeta{md}, 4, 20),
		Query:         []*queryv1.Query{query},
		Tenant:        []string{syntheticTenant},
	})
	require.NoError(t, err)
	require.Len(t, resp.Reports, 1)
	return resp
}

// TestReadAheadLatency measures queries of synthetic blocks read from a
// storage with the latency and bandwidth of an object storage.
func TestReadAheadLatency(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block syntheticBlock
	}{
		{
			name:  "small datasets",
			block: syntheticBlock{services: 9, segments: 4, profiles: 16, samples: 400, stacks: 600, functions: 1000, maxDepth: 20},
		},
		{
			name:  "large datasets",
			block: syntheticBlock{services: 1, memory: true, segments: 40, profiles: 500, samples: 200, stacks: 8000, functions: 6000, maxDepth: 24},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mem := memory.NewInMemBucket()
			md := tc.block.build(t, objstore.NewBucket(mem))
			for _, ds := range md.Datasets {
				if block.DatasetFormat(ds.Format) == block.DatasetFormat0 {
					w := block.WeightOf(ds)
					t.Logf("dataset %dK (p=%dK t=%dK s=%dK)", ds.Size>>10, w.ProfilesBytes>>10, w.TSDBBytes>>10, w.SymbolsBytes>>10)
				}
			}

			storage := &latencyBucket{Bucket: mem, ttfb: 50 * time.Millisecond, bandwidth: 200 << 20}
			reader := NewBlockReader(log.NewNopLogger(), objstore.NewBucket(storage), nil, validation.MockDefaultOverrides())

			for _, q := range []*queryv1.Query{
				{QueryType: queryv1.QueryType_QUERY_TREE, Tree: &queryv1.TreeQuery{MaxNodes: 1024}},
				{QueryType: queryv1.QueryType_QUERY_PPROF, Pprof: &queryv1.PprofQuery{MaxNodes: 1024}},
				{QueryType: queryv1.QueryType_QUERY_TIME_SERIES, TimeSeries: &queryv1.TimeSeriesQuery{Step: 1, GroupBy: []string{"service_name"}}},
			} {
				t.Run(q.QueryType.String(), func(t *testing.T) {
					var expected *queryv1.Report
					var baseline time.Duration
					for _, c := range []struct {
						rowGroups int
						datasets  int
					}{{0, 0}, {1, 0}, {2, 0}, {4, 0}, {0, 2}, {2, 2}} {
						reader.ReadAhead = block.ReadAheadOptions{RowGroups: c.rowGroups, MaxBytes: 32 << 20}
						reader.DatasetReadAhead = c.datasets
						const runs = 3
						durations := make([]time.Duration, runs)
						var requests int64
						var fetched uint64
						for i := range durations {
							requestsBefore := storage.requests.Load()
							start := time.Now()
							resp := queryBlock(t, reader, md, q, false)
							durations[i] = time.Since(start)
							requests = storage.requests.Load() - requestsBefore
							fetched = resp.Diagnostics.ExecutionNode.Stats.BytesFetched
							if expected == nil {
								expected = resp.Reports[0]
							} else {
								requireSameReport(t, expected, resp.Reports[0])
							}
						}
						slices.Sort(durations)
						median := durations[runs/2]
						if baseline == 0 {
							baseline = median
						}
						t.Logf("row groups %d, datasets %d: %7s  speedup %.2fx  requests %4d  fetched %6d KiB",
							c.rowGroups, c.datasets, median.Round(time.Millisecond), float64(baseline)/float64(median), requests, fetched>>10)
					}
				})
			}
		})
	}
}

// requireSameReport compares the reports, except for pprof reports: their
// samples are compared by stack trace, as the encoding is not deterministic.
func requireSameReport(t testing.TB, expected, actual *queryv1.Report) {
	if expected.Pprof == nil {
		require.True(t, expected.EqualVT(actual))
		return
	}
	require.Equal(t, pprofSamples(t, expected.Pprof.Pprof), pprofSamples(t, actual.Pprof.Pprof))
}

func pprofSamples(t testing.TB, b []byte) map[string]int64 {
	var p profilev1.Profile
	require.NoError(t, pprof.Unmarshal(b, &p))
	functions := make(map[uint64]string, len(p.Function))
	for _, fn := range p.Function {
		functions[fn.Id] = p.StringTable[fn.Name]
	}
	locations := make(map[uint64]string, len(p.Location))
	for _, loc := range p.Location {
		var names []string
		for _, line := range loc.Line {
			names = append(names, functions[line.FunctionId])
		}
		locations[loc.Id] = strings.Join(names, ";")
	}
	samples := make(map[string]int64)
	for _, s := range p.Sample {
		stack := make([]string, len(s.LocationId))
		for i, id := range s.LocationId {
			stack[i] = locations[id]
		}
		samples[strings.Join(stack, "|")] += s.Value[0]
	}
	return samples
}
