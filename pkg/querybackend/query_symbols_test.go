package querybackend

import (
	"context"
	"os"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/objstore/testutil"
)

func TestBlockReader_LookupSymbolServices(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	source, _ := testutil.NewFilesystemBucket(t, ctx, "../block/testdata")
	var metas metastorev1.GetBlockMetadataResponse
	raw, err := os.ReadFile("../block/testdata/block-metas.json")
	require.NoError(t, err)
	require.NoError(t, protojson.Unmarshal(raw, &metas))

	dst, tempdir := testutil.NewFilesystemBucket(t, ctx, t.TempDir())
	blocks, err := block.Compact(ctx, metas.Blocks, source,
		block.WithCompactionDestination(dst),
		block.WithCompactionTempDir(tempdir),
	)
	require.NoError(t, err)
	require.Len(t, blocks, 1)

	reader := NewBlockReader(log.NewNopLogger(), dst, nil)
	resp := invokeSymbolServices(t, ctx, reader, blocks, `{service_name="pyroscope-test/ingester", __profile_type__="memory:alloc_space:bytes:space:bytes"}`, "github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push")
	report := resp.GetReports()[0].GetSymbolServices()
	require.True(t, report.GetComplete())
	require.Equal(t, []*queryv1.SymbolServicesResult{{
		SymbolName: "github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push",
		Services: []*queryv1.SymbolService{{
			ServiceName:  "pyroscope-test/ingester",
			ProfileTypes: []string{"memory:alloc_space:bytes:space:bytes"},
		}},
	}}, report.GetResults())

	resp = invokeSymbolServices(t, ctx, reader, blocks, `{}`, "github.com/grafana/pyroscope/pkg/does.not.exist")
	report = resp.GetReports()[0].GetSymbolServices()
	require.True(t, report.GetComplete())
	require.Equal(t, []*queryv1.SymbolServicesResult{{SymbolName: "github.com/grafana/pyroscope/pkg/does.not.exist"}}, report.GetResults())

	withoutIndex := blocks[0].CloneVT()
	withoutIndex.Datasets = withoutIndex.Datasets[:len(withoutIndex.Datasets)-1]
	withoutIndex.MetadataOffset = 0
	resp = invokeSymbolServices(t, ctx, reader, []*metastorev1.BlockMeta{withoutIndex}, `{}`, "github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push")
	report = resp.GetReports()[0].GetSymbolServices()
	require.False(t, report.GetComplete())
	require.Equal(t, []*queryv1.SymbolServicesResult{{SymbolName: "github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push"}}, report.GetResults())
}

func invokeSymbolServices(t *testing.T, ctx context.Context, reader *BlockReader, blocks []*metastorev1.BlockMeta, selector string, symbolNames ...string) *queryv1.InvokeResponse {
	t.Helper()
	resp, err := reader.Invoke(ctx, &queryv1.InvokeRequest{
		Tenant:        []string{"anonymous"},
		StartTime:     blocks[0].MinTime,
		EndTime:       blocks[0].MaxTime,
		LabelSelector: selector,
		QueryPlan: &queryv1.QueryPlan{Root: &queryv1.QueryNode{
			Type:   queryv1.QueryNode_READ,
			Blocks: blocks,
		}},
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_SYMBOL_SERVICES,
			SymbolServices: &queryv1.SymbolServicesQuery{
				SymbolNames: symbolNames,
			},
		}},
	})
	require.NoError(t, err)
	require.Len(t, resp.GetReports(), 1)
	return resp
}
