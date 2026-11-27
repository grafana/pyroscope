package block

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	thanosobjstore "github.com/thanos-io/objstore"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
)

func TestCoalescedDatasetsCloseBufferedObject(t *testing.T) {
	ctx := context.Background()
	bucket := objstore.NewBucket(thanosobjstore.NewInMemBucket())
	t.Cleanup(func() { require.NoError(t, bucket.Close()) })
	md := &metastorev1.BlockMeta{
		Id:          "01J000000000000000000000A1",
		Size:        128,
		StringTable: []string{"tenant"},
		Datasets: []*metastorev1.Dataset{
			{Size: 64, TableOfContents: []uint64{0, 16, 32}},
			{Size: 64, TableOfContents: []uint64{64, 80, 96}},
		},
	}
	require.NoError(t, bucket.Upload(ctx, ObjectPath(md), bytes.NewReader(make([]byte, md.Size))))
	obj := NewObject(bucket, md)
	datasets := NewCoalescedDatasets(md.Datasets, obj, []Section{SectionProfiles})
	require.NotNil(t, datasets[0].region)
	require.Same(t, datasets[0].region, datasets[1].region)

	// No section decoding is needed to exercise region ownership. Opening
	// the small object buffers it, so neither dataset acquires its region.
	for _, ds := range datasets {
		require.NoError(t, ds.Open(ctx))
		require.NotNil(t, obj.buf)
	}
	for _, ds := range datasets {
		require.NotPanics(t, func() { require.NoError(t, ds.Close()) })
	}
	require.Nil(t, obj.buf)
}
