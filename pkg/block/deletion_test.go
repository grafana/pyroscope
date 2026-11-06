package block_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/testutil"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
)

func datasetSymbolStrings(t *testing.T, bucket objstore.Bucket, b *metastorev1.BlockMeta, ds *metastorev1.Dataset) map[string]bool {
	t.Helper()
	ctx := context.Background()
	o := block.NewObject(bucket, b)
	require.NoError(t, o.Open(ctx))
	defer func() { require.NoError(t, o.Close()) }()
	d := block.NewDataset(ds, o)
	require.NoError(t, d.Open(ctx, block.SectionProfiles, block.SectionSymbols))
	defer func() { require.NoError(t, d.Close()) }()
	partitions := make(map[uint64]bool)
	rows := make([]parquet.Row, 128)
	reader := d.ProfileRowReader()
	for {
		n, err := reader.ReadRows(rows)
		for _, row := range rows[:n] {
			partitions[schemav1.ProfileRow(row).StacktracePartitionID()] = true
		}
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}
	result := make(map[string]bool)
	for id := range partitions {
		p, err := d.Symbols().Partition(ctx, id)
		require.NoError(t, err)
		symbols := p.Symbols()
		require.NotEmpty(t, symbols.Functions)
		require.NotEmpty(t, symbols.Locations)
		for _, s := range symbols.Strings {
			result[s] = true
		}
		p.Release()
	}
	return result
}

func TestCompact_DeleteDatasets(t *testing.T) {
	ctx := context.Background()
	src, _ := testutil.NewFilesystemBucket(t, ctx, "testdata")
	raw, err := os.ReadFile("testdata/block-metas.json")
	require.NoError(t, err)
	var input metastorev1.GetBlockMetadataResponse
	require.NoError(t, protojson.Unmarshal(raw, &input))
	dst, dir := testutil.NewFilesystemBucket(t, ctx, t.TempDir())
	// Build a real L1 block containing three services, with profiles and symdb.
	merged, err := block.Compact(ctx, input.Blocks, src, block.WithCompactionDestination(dst), block.WithCompactionTempDir(dir))
	require.NoError(t, err)
	require.Len(t, merged, 1)
	original := merged[0]
	survivor := "pyroscope-test/ingester"
	keep := func(_, service string) bool { return service == survivor }
	sourceSymbols := make(map[string]bool)
	removedSymbols := make(map[string]bool)
	for _, ds := range original.Datasets {
		if ds.Name == 0 {
			continue
		}
		target := removedSymbols
		if original.StringTable[ds.Name] == survivor {
			target = sourceSymbols
		}
		for s := range datasetSymbolStrings(t, dst, original, ds) {
			target[s] = true
		}
	}
	rewritten, err := block.Compact(ctx, merged, dst, block.WithCompactionTempDir(dir), block.WithPreserveCompactionLevel(), block.WithDatasetFilter(keep))
	require.NoError(t, err)
	require.Len(t, rewritten, 1)
	b := rewritten[0]
	require.NotEqual(t, original.Id, b.Id)
	require.Equal(t, original.CompactionLevel, b.CompactionLevel)
	require.Len(t, b.Datasets, 2)
	require.Equal(t, survivor, b.StringTable[b.Datasets[0].Name])
	require.EqualValues(t, block.DatasetFormat1, b.Datasets[1].Format)
	for _, s := range b.StringTable {
		if strings.HasPrefix(s, "pyroscope-test/") {
			require.Equal(t, survivor, s)
		}
	}
	actualSymbols := datasetSymbolStrings(t, dst, b, b.Datasets[0])
	require.NotEmpty(t, actualSymbols)
	for s := range actualSymbols {
		require.True(t, sourceSymbols[s], "unexpected survivor symbol: %s", s)
	}
	uniqueRemoved := 0
	for s := range removedSymbols {
		if !sourceSymbols[s] {
			uniqueRemoved++
			require.False(t, actualSymbols[s], "deleted symbol remains: %s", s)
		}
	}
	require.Positive(t, uniqueRemoved, "fixture must contain service-specific symbols")
	// The tenant-wide TSDB index is rebuilt and contains only the kept service.
	o := block.NewObject(dst, b)
	require.NoError(t, o.Open(ctx))
	d := block.NewDataset(b.Datasets[1], o)
	require.NoError(t, d.Open(ctx, block.SectionTSDB))
	values, err := d.Index().LabelValues("service_name")
	require.NoError(t, err)
	require.Equal(t, []string{survivor}, values)
	require.NoError(t, d.Close())
	require.NoError(t, o.Close())

	t.Run("rewritten L1 block compacts cleanly to L2", func(t *testing.T) {
		// Create a second L1 block so we can merge the deletion-rewritten L1 block with it into L2.
		secondL1, err := block.Compact(ctx, input.Blocks, src, block.WithCompactionDestination(dst), block.WithCompactionTempDir(dir))
		require.NoError(t, err)
		require.Len(t, secondL1, 1)

		l2Blocks, err := block.Compact(ctx, []*metastorev1.BlockMeta{b, secondL1[0]}, dst, block.WithCompactionTempDir(dir))
		require.NoError(t, err)
		require.Len(t, l2Blocks, 1)
		require.Equal(t, uint32(2), l2Blocks[0].CompactionLevel)
	})
	t.Run("all datasets require no source reads or output", func(t *testing.T) {
		empty, _ := testutil.NewFilesystemBucket(t, ctx, t.TempDir())
		result, err := block.Compact(ctx, merged, empty, block.WithPreserveCompactionLevel(), block.WithDatasetFilter(func(string, string) bool { return false }))
		require.NoError(t, err)
		require.Empty(t, result)
	})
	t.Run("rejects preserving compaction level on L0 segments", func(t *testing.T) {
		_, err := block.Compact(ctx, input.Blocks, src, block.WithCompactionDestination(dst), block.WithCompactionTempDir(dir), block.WithPreserveCompactionLevel(), block.WithDatasetFilter(keep))
		require.ErrorContains(t, err, "cannot preserve compaction level on segments")
	})
}
