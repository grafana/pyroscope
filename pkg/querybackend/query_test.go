package querybackend

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/v2/pkg/test"
)

const cpuProfileType = "process_cpu:cpu:nanoseconds:cpu:nanoseconds"

func TestBlockContext_LookupDatasetsInMetadata_SharedSegment(t *testing.T) {
	ctx := context.Background()
	bucket := memory.NewInMemBucket()
	footer := uploadSharedSegment(t, bucket)

	for _, tc := range []struct {
		name     string
		tenants  []string
		expected []string
	}{
		{
			name:     "single tenant",
			tenants:  []string{"tenant-b"},
			expected: []string{"tenant-b/shared"},
		},
		{
			name:     "federated",
			tenants:  []string{"tenant-a", "tenant-b"},
			expected: []string{"tenant-a/shared", "tenant-b/shared"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			meta := queryMetadataResponse(footer, tc.tenants...)
			require.NotEqual(t, slices.Index(footer.StringTable, "tenant-b"), slices.Index(meta.StringTable, "tenant-b"))

			b := &blockContext{
				ctx: ctx,
				req: &request{
					src: &queryv1.InvokeRequest{Query: []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_TREE}}},
					matchers: []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, phlaremodel.LabelNameServiceName, "shared"),
						labels.MustNewMatcher(labels.MatchEqual, phlaremodel.LabelNameProfileType, cpuProfileType),
					},
				},
				obj:                       block.NewObject(&objstore.ReaderAtBucket{Bucket: bucket}, meta),
				datasetLookupFromMetadata: true,
			}
			indices := b.datasetIndices()
			require.Len(t, indices, len(tc.tenants))
			require.NoError(t, b.lookupDatasets(indices))

			md := b.obj.Metadata()
			actual := make([]string, len(md.Datasets))
			for i, ds := range md.Datasets {
				actual[i] = md.StringTable[ds.Tenant] + "/" + md.StringTable[ds.Name]
			}
			require.Equal(t, tc.expected, actual)
		})
	}
}

func uploadSharedSegment(t *testing.T, bucket *memory.InMemBucket) *metastorev1.BlockMeta {
	st := metadata.NewStringTable()
	md := &metastorev1.BlockMeta{
		FormatVersion: 1,
		Id:            test.ULID("2024-01-01T10:00:00Z"),
		Shard:         1,
		CreatedBy:     st.Put("segment-writer-0"),
	}
	for _, d := range []struct{ tenant, service string }{
		{"tenant-a", "only-a"},
		{"tenant-a", "shared"},
		{"tenant-a", ""},
		{"tenant-b", "shared"},
		{"tenant-b", ""},
		{"tenant-c", "shared"},
		{"tenant-c", ""},
	} {
		ds := &metastorev1.Dataset{Tenant: st.Put(d.tenant), Name: st.Put(d.service)}
		if d.service == "" {
			ds.Format = uint32(block.DatasetFormat1)
			ds.Labels = metadata.NewLabelBuilder(st).
				WithLabelSet(metadata.LabelNameTenantDataset, metadata.LabelValueDatasetTSDBIndex).
				Build()
		} else {
			ds.Labels = metadata.NewLabelBuilder(st).
				WithLabelSet(phlaremodel.LabelNameServiceName, d.service, phlaremodel.LabelNameProfileType, cpuProfileType).
				Build()
		}
		md.Datasets = append(md.Datasets, ds)
	}
	md.StringTable = st.Strings

	var buf bytes.Buffer
	buf.WriteString("datasets")
	md.MetadataOffset = uint64(buf.Len())
	require.NoError(t, metadata.Encode(&buf, md))
	md.Size = uint64(buf.Len())
	require.NoError(t, bucket.Upload(context.Background(), block.ObjectPath(md), bytes.NewReader(buf.Bytes())))
	return md
}

func queryMetadataResponse(footer *metastorev1.BlockMeta, tenants ...string) *metastorev1.BlockMeta {
	shard := metadata.NewStringTable()
	stored := footer.CloneVT()
	shard.Import(stored)
	resp := &metastorev1.BlockMeta{
		FormatVersion:   stored.FormatVersion,
		Id:              stored.Id,
		Tenant:          stored.Tenant,
		Shard:           stored.Shard,
		CompactionLevel: stored.CompactionLevel,
		MinTime:         stored.MinTime,
		MaxTime:         stored.MaxTime,
		CreatedBy:       stored.CreatedBy,
		MetadataOffset:  stored.MetadataOffset,
		Size:            stored.Size,
	}
	for _, ds := range stored.Datasets {
		if block.DatasetFormat(ds.Format) == block.DatasetFormat1 && slices.Contains(tenants, shard.Lookup(ds.Tenant)) {
			resp.Datasets = append(resp.Datasets, ds)
		}
	}
	shard.Export(resp)
	return resp
}
