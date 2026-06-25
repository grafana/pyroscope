package querybackend

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/objstore/testutil"
)

const (
	defaultProfileEntryIteratorBenchBlockPath = "/var/folders/n_/n694y4s176x9v42kyj3rr_tr0000gn/T/opencode/01KVST78893AT2XMBK68S1YBKA-block.bin"
	profileEntryIteratorBenchObjectPath       = "blocks/11/7874/01KVST78893AT2XMBK68S1YBKA/block.bin"
)

func BenchmarkProfileEntryIterator_RealBlock(b *testing.B) {
	ctx := context.Background()
	q, expectedRows := setupProfileEntryIteratorBenchmark(b, ctx)

	benchmarks := []struct {
		name    string
		options []profileIteratorOption
	}{
		{name: "Default"},
		{name: "GroupedServiceName", options: []profileIteratorOption{withFetchPartition(false), withGroupByLabels("service_name")}},
		{name: "AllLabelsProfileIDs", options: []profileIteratorOption{withFetchPartition(false), withAllLabels(), withFetchProfileIDs(true)}},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				it, err := profileEntryIterator(q, bm.options...)
				require.NoError(b, err)

				rows := 0
				for it.Next() {
					rows++
				}
				require.NoError(b, it.Err())
				require.NoError(b, it.Close())
				if rows != expectedRows {
					b.Fatalf("unexpected row count: got %d, want %d", rows, expectedRows)
				}
			}
		})
	}
}

func setupProfileEntryIteratorBenchmark(b *testing.B, ctx context.Context) (*queryContext, int) {
	b.Helper()

	blockPath := os.Getenv("PYROSCOPE_VALIDATE_BLOCK_PATH")
	if blockPath == "" {
		blockPath = defaultProfileEntryIteratorBenchBlockPath
	}
	if _, err := os.Stat(blockPath); err != nil {
		b.Skipf("set PYROSCOPE_VALIDATE_BLOCK_PATH to a downloaded block.bin: %v", err)
	}

	root := b.TempDir()
	require.NoError(b, linkOrCopyBenchmarkFile(blockPath, filepath.Join(root, filepath.FromSlash(profileEntryIteratorBenchObjectPath))))
	bucket, _ := testutil.NewFilesystemBucket(b, ctx, root)

	obj, err := block.NewObjectFromPath(ctx, bucket, profileEntryIteratorBenchObjectPath, block.WithObjectMaxSizeLoadInMemory(0))
	require.NoError(b, err)
	md, err := obj.ReadMetadata(ctx)
	require.NoError(b, err)
	obj.SetMetadata(md)

	dsMeta := firstProfileEntryIteratorDataset(md)
	if dsMeta == nil {
		b.Skip("block has no format0 datasets")
	}
	ds := block.NewDataset(dsMeta, obj)
	require.NoError(b, ds.Open(ctx, block.SectionTSDB, block.SectionProfiles))
	b.Cleanup(func() { require.NoError(b, ds.Close()) })

	q := &queryContext{
		blockContext: &blockContext{
			ctx: ctx,
			req: &request{
				startTime: model.Time(md.MinTime).UnixNano(),
				endTime:   model.Time(md.MaxTime).UnixNano(),
			},
		},
		ctx: ctx,
		ds:  ds,
	}

	it, err := profileEntryIterator(q)
	require.NoError(b, err)
	rows := 0
	for it.Next() {
		rows++
	}
	require.NoError(b, it.Err())
	require.NoError(b, it.Close())
	require.NotZero(b, rows)

	return q, rows
}

func firstProfileEntryIteratorDataset(md *metastorev1.BlockMeta) *metastorev1.Dataset {
	for _, ds := range md.Datasets {
		if block.DatasetFormat(ds.Format) == block.DatasetFormat0 {
			return ds
		}
	}
	return nil
}

func linkOrCopyBenchmarkFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Link(src, dst); err == nil || errors.Is(err, os.ErrExist) {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, in)
	return err
}
