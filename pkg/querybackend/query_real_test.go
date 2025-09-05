package querybackend

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/block"
	pyroobj "github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/objstore/providers/gcs"
	parquetquery "github.com/grafana/pyroscope/pkg/phlaredb/query"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func TestTimeSeries(t *testing.T) {
	logger := log.NewLogfmtLogger(os.Stderr)
	ctx := context.Background()

	bClient, err := gcs.NewBucketClient(ctx, gcs.Config{
		BucketName: "dev-us-central-0-profiles-dev-001-data",
	}, "bucket", logger)
	require.NoError(t, err)
	bucket := pyroobj.NewBucket(bClient)

	key := "blocks/1/13/01K0YNNM4CJM6W985J4KY0WPNQ/block.bin"
	obj, err := block.NewObjectFromPath(ctx, bucket, key)
	require.NoError(t, err)

	meta := obj.Metadata()
	stringTable := meta.GetStringTable()
	t.Log(
		"blockID", meta.GetId(),
		"tenant", stringTable[meta.GetTenant()],
		"compactionLevel", meta.GetCompactionLevel(),
		"shard", meta.GetShard(),
		"size", meta.GetSize(),
	)

	for idx, ds := range meta.GetDatasets() {
		t.Log(
			"idx", idx,
			"tenant", stringTable[ds.GetTenant()],
			"name", stringTable[ds.GetName()],
		)
	}

	ds := meta.GetDatasets()[0]
	idx := block.NewDataset(ds, obj)

	idx.Open(ctx)

	g, ctx := errgroup.WithContext(ctx)
	bCtx := &blockContext{
		log: logger,
		ctx: ctx,
		obj: obj,
		req: &request{
			src:       &queryv1.InvokeRequest{},
			startTime: 0,
			endTime:   time.Now().UnixNano(),
		},
		grp: g,
	}
	qCtx := bCtx.newQueryContext(meta.GetDatasets()[0])

	err = qCtx.ds.Open(ctx, block.SectionDatasetIndex, block.SectionProfiles, block.SectionDatasetIndex, block.SectionTSDB)
	require.NoError(t, err)

	entries, err := profileEntryIterator(qCtx)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, entries.Close())
	}()

	var columns v1.SampleColumns
	require.NoError(t, columns.Resolve(qCtx.ds.Profiles().Schema()))

	profiles := parquetquery.NewRepeatedRowIterator(qCtx.ctx, entries, qCtx.ds.Profiles().RowGroups(),
		columns.StacktraceID.ColumnIndex,
		columns.Value.ColumnIndex)
	defer func() {
		require.NoError(t, profiles.Close())
	}()

	for profiles.Next() {
		p := profiles.At()
		t.Log("profile at", p.Row.Timestamp)
	}

}
