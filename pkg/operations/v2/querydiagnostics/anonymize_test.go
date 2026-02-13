package querydiagnostics

import (
	"bytes"
	"context"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/querybackend"
	"github.com/grafana/pyroscope/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/pkg/test"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

const testBlockFile = "testdata/01JKT2S01VXGG0YS6TG8QC2JWD.bin"

func TestAnonymizeBlock(t *testing.T) {
	ctx := context.Background()

	origData, err := os.ReadFile(testBlockFile)
	require.NoError(t, err)

	var origMeta metastorev1.BlockMeta
	require.NoError(t, metadata.Decode(origData, &origMeta))
	origMeta.Size = uint64(len(origData))

	bucket := memory.NewInMemBucket()
	origPath := block.ObjectPath(&origMeta)
	require.NoError(t, bucket.Upload(ctx, origPath, bytes.NewReader(origData)))

	anonymizer := NewBlockAnonymizer(objstore.NewBucket(bucket))
	anonData, err := anonymizer.AnonymizeBlock(ctx, &origMeta)
	require.NoError(t, err)
	require.NotEmpty(t, anonData)

	// Decode the anonymized block metadata and assign a new ID so it
	// doesn't collide with the original in the same bucket.
	var anonMeta metastorev1.BlockMeta
	require.NoError(t, metadata.Decode(anonData, &anonMeta))
	anonMeta.Id = "01ANON0000000000000000000A"
	anonMeta.Size = uint64(len(anonData))

	anonPath := block.ObjectPath(&anonMeta)
	require.NoError(t, bucket.Upload(ctx, anonPath, bytes.NewReader(anonData)))

	t.Run("BlockStructure", func(t *testing.T) {
		testBlockStructure(t, &origMeta, &anonMeta)
	})

	t.Run("SymbolsAnonymized", func(t *testing.T) {
		testSymbolsAnonymized(t, ctx, bucket, &origMeta, &anonMeta)
	})

	t.Run("QueryTree", func(t *testing.T) {
		testQueryTree(t, ctx, bucket, &origMeta, &anonMeta)
	})

	t.Run("QueryPprof", func(t *testing.T) {
		testQueryPprof(t, ctx, bucket, &origMeta, &anonMeta)
	})

	t.Run("QueryTreeViaIndex", func(t *testing.T) {
		testQueryTreeViaIndex(t, ctx, bucket, &origMeta, &anonMeta)
	})
}

// testBlockStructure verifies the anonymized block has the same
// structural properties as the original.
func testBlockStructure(t *testing.T, orig, anon *metastorev1.BlockMeta) {
	t.Helper()
	require.NotEmpty(t, anon.Id)
	require.Equal(t, orig.Shard, anon.Shard)
	require.Equal(t, orig.CompactionLevel, anon.CompactionLevel)
	require.Equal(t, orig.FormatVersion, anon.FormatVersion)
	require.Equal(t, orig.MinTime, anon.MinTime)
	require.Equal(t, orig.MaxTime, anon.MaxTime)

	// Count Format0 and Format1 datasets.
	origF0, origF1 := countDatasetFormats(orig)
	anonF0, anonF1 := countDatasetFormats(anon)
	require.Equal(t, origF0, anonF0, "Format0 dataset count mismatch")
	require.Equal(t, origF1, anonF1, "Format1 dataset count mismatch")
}

// testSymbolsAnonymized opens the symdb of both blocks and verifies
// that all non-empty strings in the anonymized block are different
// from the original ones.
func testSymbolsAnonymized(
	t *testing.T,
	ctx context.Context,
	bucket *memory.InMemBucket,
	origMeta, anonMeta *metastorev1.BlockMeta,
) {
	t.Helper()
	origStrings := readSymbolStrings(t, ctx, bucket, origMeta)
	anonStrings := readSymbolStrings(t, ctx, bucket, anonMeta)

	require.NotEmpty(t, origStrings, "original block should have symbol strings")
	require.Equal(t, len(origStrings), len(anonStrings),
		"anonymized block should have same number of symbol strings")

	// Every non-empty original string must be replaced.
	for i, orig := range origStrings {
		if orig == "" {
			require.Empty(t, anonStrings[i], "empty string should stay empty")
			continue
		}
		require.NotEqual(t, orig, anonStrings[i],
			"string at index %d should be anonymized: %q", i, orig)
	}

	// No original string should appear anywhere in the anonymized strings.
	anonSet := make(map[string]struct{}, len(anonStrings))
	for _, s := range anonStrings {
		anonSet[s] = struct{}{}
	}
	for _, orig := range origStrings {
		if orig == "" {
			continue
		}
		_, found := anonSet[orig]
		require.False(t, found, "original string %q should not appear in anonymized output", orig)
	}
}

// testQueryTree runs a tree query on both blocks and compares the
// structural properties of the results.
func testQueryTree(
	t *testing.T,
	ctx context.Context,
	bucket *memory.InMemBucket,
	origMeta, anonMeta *metastorev1.BlockMeta,
) {
	t.Helper()
	origTree := queryTree(t, ctx, bucket, origMeta)
	anonTree := queryTree(t, ctx, bucket, anonMeta)

	require.NotNil(t, origTree)
	require.NotNil(t, anonTree)

	// The tree structure should be preserved: the same total value, the same
	// number of nodes. Function names will differ (anonymized).
	require.Equal(t, origTree.Total(), anonTree.Total(),
		"tree total value should be preserved")

	origNodes := countTreeNodes(origTree)
	anonNodes := countTreeNodes(anonTree)
	require.Equal(t, origNodes, anonNodes,
		"tree should have same number of nodes")
}

// testQueryPprof runs a pprof query on both blocks and compares the
// structural properties of the results.
func testQueryPprof(
	t *testing.T,
	ctx context.Context,
	bucket *memory.InMemBucket,
	origMeta, anonMeta *metastorev1.BlockMeta,
) {
	t.Helper()
	origProf := queryPprof(t, ctx, bucket, origMeta)
	anonProf := queryPprof(t, ctx, bucket, anonMeta)

	require.NotNil(t, origProf)
	require.NotNil(t, anonProf)

	require.Equal(t, len(origProf.Sample), len(anonProf.Sample),
		"pprof should have same number of samples")
	require.Equal(t, len(origProf.Location), len(anonProf.Location),
		"pprof should have same number of locations")
	require.Equal(t, len(origProf.Function), len(anonProf.Function),
		"pprof should have same number of functions")
	require.Equal(t, len(origProf.Mapping), len(anonProf.Mapping),
		"pprof should have same number of mappings")

	// Sample values should be identical.
	for i := range origProf.Sample {
		require.Equal(t, origProf.Sample[i].Value, anonProf.Sample[i].Value,
			"sample %d value mismatch", i)
	}

	// Function names should be anonymized.
	origFuncNames := extractFuncNames(origProf)
	anonFuncNames := extractFuncNames(anonProf)
	for _, name := range origFuncNames {
		if name == "" {
			continue
		}
		require.NotContains(t, anonFuncNames, name,
			"original function name %q should not appear in anonymized pprof", name)
	}
}

// testQueryTreeViaIndex runs a tree query using the dataset index path
// on both blocks and compares results with the direct query.
func testQueryTreeViaIndex(
	t *testing.T,
	ctx context.Context,
	bucket *memory.InMemBucket,
	origMeta, anonMeta *metastorev1.BlockMeta,
) {
	t.Helper()

	origTree := queryTreeViaIndex(t, ctx, bucket, origMeta)
	anonTree := queryTreeViaIndex(t, ctx, bucket, anonMeta)

	require.NotNil(t, origTree)
	require.NotNil(t, anonTree)

	require.Equal(t, origTree.Total(), anonTree.Total(),
		"tree total value should be preserved via index")

	origNodes := countTreeNodes(origTree)
	anonNodes := countTreeNodes(anonTree)
	require.Equal(t, origNodes, anonNodes,
		"tree should have same number of nodes via index")

	// Results should match the direct (Format0) query.
	directTree := queryTree(t, ctx, bucket, origMeta)
	require.Equal(t, directTree.Total(), origTree.Total(),
		"index query should produce same total as direct query")
	require.Equal(t, countTreeNodes(directTree), origNodes,
		"index query should produce same node count as direct query")
}

// --- helpers ---

func countDatasetFormats(meta *metastorev1.BlockMeta) (f0, f1 int) {
	for _, ds := range meta.Datasets {
		switch block.DatasetFormat(ds.Format) {
		case block.DatasetFormat0:
			f0++
		case block.DatasetFormat1:
			f1++
		}
	}
	return
}

func readSymbolStrings(
	t *testing.T,
	ctx context.Context,
	bucket *memory.InMemBucket,
	meta *metastorev1.BlockMeta,
) []string {
	t.Helper()
	wrapped := objstore.NewBucket(bucket)

	obj := block.NewObject(wrapped, meta)
	require.NoError(t, obj.Open(ctx))
	defer func() {
		require.NoError(t, obj.Close())
	}()

	fullMeta, err := obj.ReadMetadata(ctx)
	require.NoError(t, err)
	obj.SetMetadata(fullMeta)

	for _, dsMeta := range fullMeta.Datasets {
		if block.DatasetFormat(dsMeta.Format) != block.DatasetFormat0 {
			continue
		}
		ds := block.NewDataset(dsMeta, obj)
		require.NoError(t, ds.Open(ctx, block.SectionSymbols))

		reader, ok := ds.Symbols().(*symdb.Reader)
		require.True(t, ok)

		p, err := reader.Partition(ctx, 0)
		require.NoError(t, err)

		var strings = slices.Clone(p.Symbols().Strings)
		p.Release()
		require.NoError(t, ds.Close())

		return strings
	}

	t.Fatal("no Format0 dataset found")
	return nil
}

func invokeQuery(
	t *testing.T,
	ctx context.Context,
	bucket *memory.InMemBucket,
	meta *metastorev1.BlockMeta,
	queries []*queryv1.Query,
) *queryv1.InvokeResponse {
	t.Helper()
	logger := test.NewTestingLogger(t)
	wrapped := objstore.NewBucket(bucket)
	reader := querybackend.NewBlockReader(logger, wrapped, nil)

	plan := queryplan.Build([]*metastorev1.BlockMeta{meta}, 4, 20)

	resp, err := reader.Invoke(ctx, &queryv1.InvokeRequest{
		StartTime:     meta.MinTime,
		EndTime:       meta.MaxTime,
		LabelSelector: "{service_name=\"pyroscope\"}",
		QueryPlan:     plan,
		Query:         queries,
		Tenant:        []string{"anonymous"},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Reports, 1)
	return resp
}

func queryTree(
	t *testing.T,
	ctx context.Context,
	bucket *memory.InMemBucket,
	meta *metastorev1.BlockMeta,
) *phlaremodel.Tree {
	t.Helper()

	// Build query plan from the block's Format0 datasets only
	// (mimicking what the query frontend sends for filtered queries).
	planMeta := meta.CloneVT()
	planMeta.Datasets = slices.DeleteFunc(planMeta.Datasets, func(ds *metastorev1.Dataset) bool {
		return block.DatasetFormat(ds.Format) == block.DatasetFormat1
	})

	resp := invokeQuery(t, ctx, bucket, planMeta,
		[]*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{MaxNodes: 16384},
		}},
	)
	tree, err := phlaremodel.UnmarshalTree(resp.Reports[0].Tree.Tree)
	require.NoError(t, err)
	return tree
}

func queryPprof(
	t *testing.T,
	ctx context.Context,
	bucket *memory.InMemBucket,
	meta *metastorev1.BlockMeta,
) *profilev1.Profile {
	t.Helper()

	// Build query plan from the block's Format0 datasets only
	// (mimicking what the query frontend sends for filtered queries).
	planMeta := meta.CloneVT()
	planMeta.Datasets = slices.DeleteFunc(planMeta.Datasets, func(ds *metastorev1.Dataset) bool {
		return block.DatasetFormat(ds.Format) == block.DatasetFormat1
	})

	resp := invokeQuery(t, ctx, bucket, planMeta,
		[]*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_PPROF,
			Pprof:     &queryv1.PprofQuery{},
		}},
	)
	var p profilev1.Profile
	require.NoError(t, pprof.Unmarshal(resp.Reports[0].Pprof.Pprof, &p))
	return &p
}

func extractFuncNames(p *profilev1.Profile) []string {
	names := make([]string, len(p.Function))
	for i, f := range p.Function {
		names[i] = p.StringTable[f.Name]
	}
	return names
}

func countTreeNodes(tree *phlaremodel.Tree) int {
	n := 0
	tree.IterateStacks(func(_ string, _ int64, _ []string) {
		n++
	})
	return n
}

func queryTreeViaIndex(
	t *testing.T,
	ctx context.Context,
	bucket *memory.InMemBucket,
	meta *metastorev1.BlockMeta,
) *phlaremodel.Tree {
	t.Helper()

	// Keep only Format1 datasets, triggering the dataset index code path.
	planMeta := meta.CloneVT()
	planMeta.Datasets = slices.DeleteFunc(planMeta.Datasets, func(ds *metastorev1.Dataset) bool {
		return block.DatasetFormat(ds.Format) == block.DatasetFormat0
	})

	resp := invokeQuery(t, ctx, bucket, planMeta,
		[]*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{MaxNodes: 16384},
		}},
	)
	tree, err := phlaremodel.UnmarshalTree(resp.Reports[0].Tree.Tree)
	require.NoError(t, err)
	return tree
}
