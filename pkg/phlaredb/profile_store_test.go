package phlaredb

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
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
	p           schemav1.Profile
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
	tp.p.Samples = []*schemav1.Sample{
		{
			StacktraceID: 0x1,
			Value:        10.0,
		},
	}
	tp.populateFingerprint()

	return tp
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
	_, err = reader.Read(slice)
	require.NoError(t, err)

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
			name:            "multiple row groups because of maximum size",
			cfg:             &ParquetConfig{MaxRowGroupBytes: 1828, MaxBufferRowCount: 100000},
			expectedNumRGs:  10,
			expectedNumRows: 100,
			values:          sameProfileStream,
		},
		{
			name:            "multiple row groups because of maximum row num",
			cfg:             &ParquetConfig{MaxRowGroupBytes: 128000, MaxBufferRowCount: 10},
			expectedNumRGs:  10,
			expectedNumRows: 100,
			values:          sameProfileStream,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := t.TempDir()
			require.NoError(t, store.Init(path, tc.cfg))

			for i := 0; i < 100; i++ {
				p := tc.values(i)
				require.NoError(t, store.ingest(ctx, []*schemav1.Profile{&p.p}, p.lbls, p.profileName, emptyRewriter()))
			}

			// ensure the correct number of files are created
			numRows, numRGs, err := store.Flush(context.Background())
			require.NoError(t, err)
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
			assert.Equal(t, "00000000-0000-0000-0000-000000000000", rows[0].ID.String())
			assert.Equal(t, "00000000-0000-0000-0000-000000000001", rows[1].ID.String())
			assert.Equal(t, "00000000-0000-0000-0000-000000000002", rows[2].ID.String())

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
	require.NoError(t, store.Init(path, defaultParquetConfig))

	for i := 0; i < 9; i++ {
		p := threeProfileStreams(i)
		require.NoError(t, store.ingest(ctx, []*schemav1.Profile{&p.p}, p.lbls, p.profileName, emptyRewriter()))
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
		head, err = NewHead(ctx, cfg)
	)
	require.NoError(t, err)

	// force different row group segements for profiles
	head.profiles.cfg = &ParquetConfig{MaxRowGroupBytes: 128000, MaxBufferRowCount: 3}

	for i := 0; i < 9; i++ {
		require.NoError(t, ingestThreeProfileStreams(ctx, i, head.Ingest))
	}

	// now query the store
	params := &ingestv1.SelectProfilesRequest{
		Start:         0,
		End:           1000000000000,
		LabelSelector: "{}",
		Type: &typesv1.ProfileType{
			Name:       "process_cpu",
			SampleType: "cpu",
			SampleUnit: "nanoseconds",
			PeriodType: "cpu",
			PeriodUnit: "nanoseconds",
		},
	}

	t.Run("select matching profiles", func(t *testing.T) {
		pIt, err := head.SelectMatchingProfiles(ctx, params)
		require.NoError(t, err)

		// ensure we see the profiles we expect
		var profileTS []int64
		for pIt.Next() {
			profileTS = append(profileTS, pIt.At().Timestamp().Unix())
		}
		assert.Equal(t, []int64{0, 1, 2, 3, 4, 5, 6, 7, 8}, profileTS)
	})

	t.Run("merge by labels", func(t *testing.T) {
		pIt, err := head.SelectMatchingProfiles(ctx, params)
		require.NoError(t, err)
		result, err := head.MergeByLabels(ctx, pIt, "stream")
		require.NoError(t, err)
		// expect 3 series
		require.Len(t, result, 3)

		// expect all ts to be there
		var profileTS []int64
		for _, s := range result {
			for _, p := range s.Points {
				profileTS = append(profileTS, model.Time(p.Timestamp).Unix())
			}
		}
		assert.ElementsMatch(t, []int64{0, 1, 2, 3, 4, 5, 6, 7, 8}, profileTS)
	})

	t.Run("merge by stacktraces", func(t *testing.T) {
		pIt, err := head.SelectMatchingProfiles(ctx, params)
		require.NoError(t, err)
		result, err := head.MergeByStacktraces(ctx, pIt)
		require.NoError(t, err)

		var values []int64
		for _, s := range result.Stacktraces {
			values = append(values, s.Value)
		}
		assert.ElementsMatch(t, []int64{90, 180}, values)
	})

	t.Run("merge by pprof", func(t *testing.T) {
		pIt, err := head.SelectMatchingProfiles(ctx, params)
		require.NoError(t, err)
		result, err := head.MergePprof(ctx, pIt)
		require.NoError(t, err)

		var values []int64
		for _, s := range result.Sample {
			values = append(values, s.Value...)
		}
		assert.ElementsMatch(t, []int64{90, 180}, values)
	})

}
