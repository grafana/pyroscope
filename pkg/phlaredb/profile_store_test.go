package phlaredb

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/google/pprof/profile"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
	"github.com/grafana/phlare/pkg/pprof/testhelper"
)

func testContext(t testing.TB) context.Context {
	logger := log.NewNopLogger()
	if testing.Verbose() {
		logger = log.NewLogfmtLogger(os.Stderr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ctx = phlarecontext.WithLogger(ctx, logger)

	reg := prometheus.NewPedanticRegistry()
	ctx = phlarecontext.WithRegistry(ctx, reg)
	ctx = contextWithHeadMetrics(ctx, newHeadMetrics(reg))

	return ctx
}

type testProfile struct {
	p           schemav1.InMemoryProfile
	profileName string
	lbls        phlaremodel.Labels
}

func (tp *testProfile) populateFingerprint() {
	lbls := phlaremodel.NewLabelsBuilder(tp.lbls)
	lbls.Set(model.MetricNameLabel, tp.profileName)
	tp.p.SeriesFingerprint = model.Fingerprint(lbls.Labels().Hash())
}

func sameProfileStream(i int) *testProfile {
	tp := &testProfile{}

	tp.profileName = "process_cpu:cpu:nanoseconds:cpu:nanoseconds"
	tp.lbls = phlaremodel.LabelsFromStrings(
		phlaremodel.LabelNameProfileType, tp.profileName,
		"job", "test",
	)

	tp.p.ID = uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i))
	tp.p.TimeNanos = time.Second.Nanoseconds() * int64(i)

	tp.p.Samples = schemav1.Samples{
		StacktraceIDs: []uint32{0x1},
		Values:        []uint64{10},
	}
	tp.populateFingerprint()

	return tp
}

// This will simulate a profile stream which ends and a new one starts at i > boundary
func profileStreamEndingAndStarting(boundary int) func(int) *testProfile {
	return func(i int) *testProfile {
		tp := &testProfile{}

		series := "at-beginning"
		if i > boundary {
			series = "at-end"
		}

		tp.profileName = "process_cpu:cpu:nanoseconds:cpu:nanoseconds"
		tp.lbls = phlaremodel.LabelsFromStrings(
			phlaremodel.LabelNameProfileType, tp.profileName,
			"job", "test",
			"stream", series,
		)

		tp.p.ID = uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i))
		tp.p.TimeNanos = time.Second.Nanoseconds() * int64(i)
		tp.p.Samples = schemav1.Samples{
			StacktraceIDs: []uint32{0x1},
			Values:        []uint64{10},
		}
		tp.populateFingerprint()
		return tp
	}
}

func nProfileStreams(n int) func(int) *testProfile {
	return func(i int) *testProfile {
		tp := sameProfileStream(i / n)

		tp.lbls = phlaremodel.LabelsFromStrings(
			phlaremodel.LabelNameProfileType, tp.profileName,
			"job", "test",
			"stream", fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%d", i%n)))),
		)
		tp.p.ID = uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i))

		tp.populateFingerprint()
		return tp
	}
}

func readFullParquetFile[M any](t *testing.T, path string) ([]M, uint64) {
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
	}()
	stat, err := f.Stat()
	require.NoError(t, err)

	pf, err := parquet.OpenFile(f, stat.Size())
	require.NoError(t, err)
	numRGs := uint64(len(pf.RowGroups()))

	reader := parquet.NewGenericReader[M](f)

	slice := make([]M, reader.NumRows())
	offset := 0
	for {
		n, err := reader.Read(slice[offset:])
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		offset += n
	}

	return slice, numRGs
}

// TestProfileStore_RowGroupSplitting tests that the profile store splits row
// groups when certain limits are reached. It also checks that on flushing the
// block is aggregated correctly. All ingestion is done using the same profile series.
func TestProfileStore_RowGroupSplitting(t *testing.T) {
	var (
		ctx   = testContext(t)
		store = newProfileStore(ctx)
	)

	for _, tc := range []struct {
		name            string
		cfg             *ParquetConfig
		expectedNumRows uint64
		expectedNumRGs  uint64
		values          func(int) *testProfile
	}{
		{
			name:            "single row group",
			cfg:             defaultParquetConfig,
			expectedNumRGs:  1,
			expectedNumRows: 100,
			values:          sameProfileStream,
		},
		{
			name:            "a stream ending after half of the samples and a new one starting",
			cfg:             &ParquetConfig{MaxRowGroupBytes: 128000, MaxBufferRowCount: 10},
			expectedNumRGs:  10,
			expectedNumRows: 100,
			values:          profileStreamEndingAndStarting(50),
		},
		{
			name:            "multiple row groups because of maximum row num",
			cfg:             &ParquetConfig{MaxRowGroupBytes: 128000, MaxBufferRowCount: 10},
			expectedNumRGs:  10,
			expectedNumRows: 100,
			values:          sameProfileStream,
		},
		{
			name:            "a single sample per series",
			cfg:             &ParquetConfig{MaxRowGroupBytes: 128000, MaxBufferRowCount: 10},
			expectedNumRGs:  10,
			expectedNumRows: 100,
			values:          nProfileStreams(100),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := t.TempDir()
			require.NoError(t, store.Init(path, tc.cfg, newHeadMetrics(prometheus.NewRegistry())))

			for i := 0; i < 100; i++ {
				p := tc.values(i)
				require.NoError(t, store.ingest(ctx, []schemav1.InMemoryProfile{p.p}, p.lbls, p.profileName, emptyRewriter()))
				for store.flushing.Load() {
					time.Sleep(time.Millisecond)
				}
			}

			// ensure the correct number of files are created
			numRows, numRGs, err := store.Flush(context.Background())
			require.NoError(t, err)
			require.NoError(t, store.DeleteRowGroups())
			assert.Equal(t, tc.expectedNumRows, numRows)
			assert.Equal(t, tc.expectedNumRGs, numRGs)

			// list folder to ensure only aggregted block exists
			files, err := os.ReadDir(path)
			require.NoError(t, err)
			require.Equal(t, []string{"index.tsdb", "profiles.parquet"}, lo.Map(files, func(e os.DirEntry, _ int) string {
				return e.Name()
			}))

			rows, numRGs := readFullParquetFile[*schemav1.Profile](t, path+"/profiles.parquet")
			require.Equal(t, int(tc.expectedNumRows), len(rows))
			assert.Equal(t, tc.expectedNumRGs, numRGs)

			// ensure all profiles are there
			idExisting := make(map[uuid.UUID]int, tc.expectedNumRows)
			for i := range rows {
				_, ok := idExisting[rows[i].ID]
				assert.False(t, ok, "expected ID to not exists more than once")
				idExisting[rows[i].ID] = i
			}
			for i := 0; i < int(tc.expectedNumRows); i++ {
				id := uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i))
				_, ok := idExisting[id]
				assert.True(t, ok, fmt.Sprintf("expected ID %s to exist in output", id.String()))
			}
		})
	}
}

var streams = []string{"stream-a", "stream-b", "stream-c"}

func threeProfileStreams(i int) *testProfile {
	tp := sameProfileStream(i)

	lbls := phlaremodel.NewLabelsBuilder(tp.lbls)
	lbls.Set("stream", streams[i%3])
	tp.lbls = lbls.Labels()
	tp.populateFingerprint()
	return tp
}

// TestProfileStore_Ingestion_SeriesIndexes during ingestion, the profile store
// writes out row groups to disk temporarily. Later when finishing up the block
// it will have to combine those files on disk and update the seriesIndex,
// which is only known when the TSDB index is written to disk.
func TestProfileStore_Ingestion_SeriesIndexes(t *testing.T) {
	var (
		ctx   = testContext(t)
		store = newProfileStore(ctx)
	)
	path := t.TempDir()
	require.NoError(t, store.Init(path, defaultParquetConfig, newHeadMetrics(prometheus.NewRegistry())))

	for i := 0; i < 9; i++ {
		p := threeProfileStreams(i)
		require.NoError(t, store.ingest(ctx, []schemav1.InMemoryProfile{p.p}, p.lbls, p.profileName, emptyRewriter()))
	}

	// flush profiles and ensure the correct number of files are created
	numRows, numRGs, err := store.Flush(context.Background())
	require.NoError(t, err)
	assert.Equal(t, uint64(9), numRows)
	assert.Equal(t, uint64(1), numRGs)

	// now compare the written parquet files
	rows, numRGs := readFullParquetFile[*schemav1.Profile](t, path+"/profiles.parquet")
	require.Equal(t, 9, len(rows))
	assert.Equal(t, uint64(1), numRGs)
	// expected in series ID order and then by timeNanos
	for i := 0; i < 9; i++ {
		id := i%3*3 + i/3 // generates 0,3,6,1,4,7,2,5,8
		assert.Equal(t, fmt.Sprintf("00000000-0000-0000-0000-%012d", id), rows[i].ID.String())
		assert.Equal(t, uint32(i/3), rows[i].SeriesIndex)
	}
}

func BenchmarkFlush(b *testing.B) {
	b.StopTimer()
	ctx := testContext(b)
	metrics := newHeadMetrics(prometheus.NewRegistry())
	rw := emptyRewriter()
	b.ReportAllocs()
	samples := schemav1.Samples{
		Values:        make([]uint64, 10000),
		StacktraceIDs: make([]uint32, 10000),
	}
	for i := 0; i < 10000; i++ {
		samples.Values[i] = uint64(i)
		samples.StacktraceIDs[i] = uint32(i)
	}
	for i := 0; i < b.N; i++ {

		path := b.TempDir()
		store := newProfileStore(ctx)
		require.NoError(b, store.Init(path, defaultParquetConfig, metrics))
		for rg := 0; rg < 10; rg++ {
			for i := 0; i < 10^6; i++ {
				p := threeProfileStreams(i)
				p.p.Samples = samples
				require.NoError(b, store.ingest(ctx, []schemav1.InMemoryProfile{p.p}, p.lbls, p.profileName, rw))
			}
			require.NoError(b, store.cutRowGroup(len(store.slice)))
		}
		b.StartTimer()
		_, _, err := store.Flush(context.Background())
		require.NoError(b, err)
		b.StopTimer()
	}
}

func ingestThreeProfileStreams(ctx context.Context, i int, ingest func(context.Context, *profilev1.Profile, uuid.UUID, ...*typesv1.LabelPair) error) error {
	p := testhelper.NewProfileBuilder(time.Second.Nanoseconds() * int64(i))
	p.CPUProfile()
	p.WithLabels(
		"job", "foo",
		"stream", streams[i%3],
	)
	p.UUID = uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i))
	p.ForStacktraceString("func1", "func2").AddSamples(10)
	p.ForStacktraceString("func1").AddSamples(20)

	return ingest(ctx, p.Profile, p.UUID, p.Labels...)
}

// TestProfileStore_Querying
func TestProfileStore_Querying(t *testing.T) {
	var (
		ctx = testContext(t)
		cfg = Config{
			DataPath: t.TempDir(),
		}
		head, err = NewHead(ctx, cfg, NoLimit)
	)
	require.NoError(t, err)

	// force different row group segements for profiles
	head.profiles.cfg = &ParquetConfig{MaxRowGroupBytes: 128000, MaxBufferRowCount: 3}

	for i := 0; i < 9; i++ {
		require.NoError(t, ingestThreeProfileStreams(ctx, i, func(ctx context.Context, p *profilev1.Profile, u uuid.UUID, lp ...*typesv1.LabelPair) error {
			defer func() {
				// wait for the profile to be flushed
				// todo(cyriltovena): We shouldn't need this, but when calling head.Queriers(), flushing row group and then querying using the queriers previously returned we will miss the new headDiskQuerier.
				for head.profiles.flushing.Load() {
					time.Sleep(time.Millisecond)
				}
			}()
			return head.Ingest(ctx, p, u, lp...)
		}))
	}

	// now query the store
	params := &ingestv1.SelectProfilesRequest{
		Start:         0,
		End:           1000000000000,
		LabelSelector: "{}",
		Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
	}

	t.Run("select matching profiles", func(t *testing.T) {
		pIt, err := head.Queriers().SelectMatchingProfiles(ctx, params)
		require.NoError(t, err)

		// ensure we see the profiles we expect
		var profileTS []int64
		for pIt.Next() {
			profileTS = append(profileTS, pIt.At().Timestamp().Unix())
		}
		assert.Equal(t, []int64{0, 1, 2, 3, 4, 5, 6, 7, 8}, profileTS)
	})

	t.Run("merge by labels", func(t *testing.T) {
		client, cleanup := head.Queriers().ingesterClient()
		defer cleanup()

		bidi := client.MergeProfilesLabels(ctx)

		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesLabelsRequest{
			Request: &ingestv1.SelectProfilesRequest{
				LabelSelector: params.LabelSelector,
				Type:          params.Type,
				Start:         params.Start,
				End:           params.End,
			},
			By: []string{"stream"},
		}))

		for {
			resp, err := bidi.Receive()
			require.NoError(t, err)

			// when empty, finished reading profiles
			if resp.SelectedProfiles == nil {
				break
			}

			selectProfiles := make([]bool, len(resp.SelectedProfiles.Profiles))
			for pos := range resp.SelectedProfiles.Profiles {
				selectProfiles[pos] = true
			}

			require.NoError(t, bidi.Send(&ingestv1.MergeProfilesLabelsRequest{
				Profiles: selectProfiles,
			}))
		}

		// still receiving a result
		result, err := bidi.Receive()
		require.NoError(t, err)

		streams := []string{}
		timestamps := []int64{}
		values := []float64{}
		for _, x := range result.Series {
			streams = append(streams, phlaremodel.LabelPairsString(x.Labels))
			for _, p := range x.Points {
				timestamps = append(timestamps, p.Timestamp)
				values = append(values, p.Value)
			}
		}
		assert.Equal(
			t,
			[]string{`{stream="stream-a"}`, `{stream="stream-b"}`, `{stream="stream-c"}`},
			streams,
		)
		assert.Equal(
			t,
			[]int64{0, 3000, 6000, 1000, 4000, 7000, 2000, 5000, 8000},
			timestamps,
		)
		assert.Equal(
			t,
			[]float64{30, 30, 30, 30, 30, 30, 30, 30, 30},
			values,
		)
	})

	t.Run("merge by stacktraces", func(t *testing.T) {
		client, cleanup := head.Queriers().ingesterClient()
		defer cleanup()

		bidi := client.MergeProfilesStacktraces(ctx)

		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{
			Request: &ingestv1.SelectProfilesRequest{
				LabelSelector: params.LabelSelector,
				Type:          params.Type,
				Start:         params.Start,
				End:           params.End,
			},
		}))

		for {
			resp, err := bidi.Receive()
			require.NoError(t, err)

			// when empty, finished reading profiles
			if resp.SelectedProfiles == nil {
				break
			}

			selectProfiles := make([]bool, len(resp.SelectedProfiles.Profiles))
			for pos := range resp.SelectedProfiles.Profiles {
				selectProfiles[pos] = true
			}

			require.NoError(t, bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{
				Profiles: selectProfiles,
			}))
		}

		// still receiving a result
		result, err := bidi.Receive()
		require.NoError(t, err)

		at, err := phlaremodel.UnmarshalTree(result.Result.TreeBytes)
		require.NoError(t, err)

		et := new(phlaremodel.Tree)
		et.InsertStack(90, "func2", "func1")
		et.InsertStack(180, "func1")

		assert.Equal(t, et.String(), at.String())
	})

	t.Run("merge by pprof", func(t *testing.T) {
		client, cleanup := head.Queriers().ingesterClient()
		defer cleanup()

		bidi := client.MergeProfilesPprof(ctx)

		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesPprofRequest{
			Request: &ingestv1.SelectProfilesRequest{
				LabelSelector: params.LabelSelector,
				Type:          params.Type,
				Start:         params.Start,
				End:           params.End,
			},
		}))

		for {
			resp, err := bidi.Receive()
			require.NoError(t, err)

			// when empty, finished reading profiles
			if resp.SelectedProfiles == nil {
				break
			}

			selectProfiles := make([]bool, len(resp.SelectedProfiles.Profiles))
			for pos := range resp.SelectedProfiles.Profiles {
				selectProfiles[pos] = true
			}

			require.NoError(t, bidi.Send(&ingestv1.MergeProfilesPprofRequest{
				Profiles: selectProfiles,
			}))
		}

		// still receiving a result
		result, err := bidi.Receive()
		require.NoError(t, err)

		var (
			values = make(map[string]int64)
			sb     strings.Builder
		)

		p, err := profile.ParseUncompressed(result.Result)
		require.NoError(t, err)

		for _, x := range p.Sample {
			sb.Reset()
			for _, loc := range x.Location {
				for _, line := range loc.Line {
					sb.WriteString(line.Function.Name)
					sb.WriteString("/")
				}
			}
			stacktrace := sb.String()[:sb.Len()-1]
			values[stacktrace] = x.Value[0]
		}
		assert.Equal(
			t,
			map[string]int64{"func1/func2": 90, "func1": 180},
			values,
		)
	})
}
