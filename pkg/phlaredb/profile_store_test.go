package phlaredb

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	phlaremodel "github.com/grafana/phlare/pkg/model"
	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
)

func testContext(t *testing.T) context.Context {
	logger := log.NewNopLogger()
	if testing.Verbose() {
		logger = log.NewLogfmtLogger(os.Stderr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	return phlarecontext.WithLogger(ctx, logger)
}

type testProfile struct {
	p           schemav1.Profile
	profileName string
	lbls        phlaremodel.Labels
}

func sameProfileStream(i int) *testProfile {
	tp := &testProfile{}

	tp.p.ID = uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i))
	tp.p.TimeNanos = time.Second.Nanoseconds() * int64(i)
	tp.lbls = phlaremodel.LabelsFromStrings("job", "test")
	tp.profileName = "test"

	return tp
}

func readFullParquetFile[M any](t *testing.T, path string) ([]M, uint64) {
	f, err := os.Open(path)
	require.NoError(t, err)
	defer require.NoError(t, f.Close())
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

func TestProfileStore_Ingestion(t *testing.T) {
	// create index
	index, err := newProfileIndex(32, newHeadMetrics(nil))
	require.NoError(t, err)

	var (
		ctx   = testContext(t)
		store = newProfileStore(ctx, defaultParquetConfig, index)
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
			cfg:             &ParquetConfig{MaxRowGroupBytes: 1280, MaxBufferRowCount: 100000},
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
			numRows, numRGs, err := store.Flush()
			require.NoError(t, err)
			assert.Equal(t, tc.expectedNumRows, numRows)
			assert.Equal(t, tc.expectedNumRGs, numRGs)

			// list folder to ensure only aggregted block exists
			files, err := os.ReadDir(path)
			require.NoError(t, err)
			require.Equal(t, []string{"profiles.parquet"}, lo.Map(files, func(e os.DirEntry, _ int) string {
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
