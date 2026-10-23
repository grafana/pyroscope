package querybackend

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/go-kit/log"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

// This fixture is small, but exercises BlockReader.Invoke, including Format1
// dataset-index lookup, unlike the symdb-only reuse experiments.
func BenchmarkBlockReaderTreeResultCache(b *testing.B) {
	bucket := memory.NewInMemBucket()
	var blocks []*metastorev1.BlockMeta
	err := filepath.WalkDir("testdata/samples", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		var md metastorev1.BlockMeta
		if decodeErr := metadata.Decode(data, &md); decodeErr != nil {
			return decodeErr
		}
		md.Size = uint64(len(data))
		blocks = append(blocks, &md)
		return bucket.Upload(context.Background(), block.ObjectPath(&md), bytes.NewReader(data))
	})
	if err != nil {
		b.Fatal(err)
	}
	for _, md := range blocks {
		if slices.ContainsFunc(md.Datasets, func(d *metastorev1.Dataset) bool { return block.DatasetFormat(d.Format) == block.DatasetFormat1 }) {
			md.Datasets = slices.DeleteFunc(md.Datasets, func(d *metastorev1.Dataset) bool { return d.Format == 0 })
		}
	}
	req := &queryv1.InvokeRequest{
		EndTime: time.Now().UnixMilli(), LabelSelector: "{}", Tenant: []string{"anonymous"},
		QueryPlan: queryplan.Build(blocks, 10, 10),
		Query:     []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_TREE, Tree: &queryv1.TreeQuery{MaxNodes: 16}}},
	}
	storage := &objstore.ReaderAtBucket{Bucket: bucket}
	uncached := NewBlockReader(log.NewNopLogger(), storage, nil, validation.MockDefaultOverrides())
	cached := NewBlockReader(log.NewNopLogger(), storage, nil, validation.MockDefaultOverrides(), WithTreeResultCacheMaxBytes(8<<20, nil))
	first, err := uncached.Invoke(context.Background(), req.CloneVT())
	if err != nil {
		b.Fatal(err)
	}
	warm, err := cached.Invoke(context.Background(), req.CloneVT())
	if err != nil {
		b.Fatal(err)
	}
	if len(first.Reports) == 0 || len(warm.Reports) == 0 || !proto.Equal(first.Reports[0], warm.Reports[0]) {
		b.Fatal("warmup result differs from uncached result")
	}
	for _, tc := range []struct {
		name   string
		reader *BlockReader
	}{{"uncached", uncached}, {"warm_hit", cached}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := tc.reader.Invoke(context.Background(), req.CloneVT()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
