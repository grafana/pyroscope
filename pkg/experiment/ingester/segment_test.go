package ingester

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
	phlarecontext "github.com/grafana/pyroscope/pkg/phlare/context"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	pprofth "github.com/grafana/pyroscope/pkg/pprof/testhelper"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	model2 "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type metastoreClient struct {
	AddBlock_ func(ctx context.Context, in *metastorev1.AddBlockRequest, opts ...grpc.CallOption) (*metastorev1.AddBlockResponse, error)
}

func (m *metastoreClient) AddBlock(ctx context.Context, in *metastorev1.AddBlockRequest, opts ...grpc.CallOption) (*metastorev1.AddBlockResponse, error) {
	return m.AddBlock_(ctx, in, opts...)
}

func (m *metastoreClient) ListBlocksForQuery(ctx context.Context, in *metastorev1.ListBlocksForQueryRequest, opts ...grpc.CallOption) (*metastorev1.ListBlocksForQueryResponse, error) {
	panic("implement me")
}

func (m *metastoreClient) ReadIndex(ctx context.Context, in *metastorev1.ReadIndexRequest, opts ...grpc.CallOption) (*metastorev1.ReadIndexResponse, error) {
	panic("implement me")
}

const testSVCName = "svc239"
const testTenant = "tenant42"
const testShard = shardKey(239)

func testProfile() *pprofth.ProfileBuilder {
	return pprofth.NewProfileBuilder(time.Now().UnixNano()).
		CPUProfile().
		WithLabels(model.LabelNameServiceName, testSVCName).
		ForStacktraceString("foo", "bar").
		AddSamples(1)
}

var cpuProfileType = &typesv1.ProfileType{
	ID:         "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
	Name:       "process_cpu",
	SampleType: "cpu",
	SampleUnit: "nanoseconds",
	PeriodType: "cpu",
	PeriodUnit: "nanoseconds",
}

func TestSegmentIngest(t *testing.T) {
	sw := newTestSegmentWriter(t)
	defer sw.Stop()
	blocks := make(chan *metastorev1.BlockMeta, 1)
	sw.client.AddBlock_ = func(ctx context.Context, in *metastorev1.AddBlockRequest, opts ...grpc.CallOption) (*metastorev1.AddBlockResponse, error) {
		blocks <- in.Block
		close(blocks)
		return &metastorev1.AddBlockResponse{}, nil
	}
	awaiter, err := sw.ingest(testShard, func(head segmentIngest) error {
		p := testProfile()
		err := head.ingest(context.Background(), testTenant, p.Profile, p.UUID, p.Labels)
		assert.NoError(t, err)
		return nil
	})
	assert.NoError(t, err)

	err = awaiter.waitFlushed(context.Background())
	assert.NoError(t, err)

	meta := <-blocks
	assert.Len(t, meta.TenantServices, 1)
	assert.Equal(t, uint32(testShard), meta.Shard)
	assert.Equal(t, testTenant, meta.TenantServices[0].TenantId)
	assert.Equal(t, testSVCName, meta.TenantServices[0].Name)

	blockQuerier := sw.createBlockFromMeta(meta, meta.TenantServices[0])

	q := blockQuerier.Queriers()
	err = q.Open(context.Background())
	assert.NoError(t, err)

	res, err := q[0].SelectMergeByStacktraces(context.Background(), &ingestv1.SelectProfilesRequest{
		LabelSelector: fmt.Sprintf("{%s=\"%s\"}", model.LabelNameServiceName, testSVCName),
		Type:          cpuProfileType,
		Start:         0,
		End:           time.Now().UnixMilli(),
	}, 100)
	assert.NoError(t, err)
	collapsed := bytes.NewBuffer(nil)
	res.WriteCollapsed(collapsed)
	assert.Equal(t, string(collapsed.String()), "bar;foo 1\n")
}

func TestSegmentIngestDLQ(t *testing.T) {
	sw := newTestSegmentWriter(t)
	defer sw.Stop()
	sw.client.AddBlock_ = func(ctx context.Context, in *metastorev1.AddBlockRequest, opts ...grpc.CallOption) (*metastorev1.AddBlockResponse, error) {
		return nil, fmt.Errorf("metastore unavailable")
	}
	awaiter, err := sw.ingest(testShard, func(head segmentIngest) error {
		p := testProfile()
		err := head.ingest(context.Background(), testTenant, p.Profile, p.UUID, p.Labels)
		assert.NoError(t, err)
		return nil
	})
	assert.NoError(t, err)

	err = awaiter.waitFlushed(context.Background())
	assert.NoError(t, err)

	metas := getMetadataDLQ(sw)
	assert.Len(t, metas, 1)
	meta := metas[0]

	assert.Len(t, meta.TenantServices, 1)
	assert.Equal(t, uint32(testShard), meta.Shard)
	assert.Equal(t, testTenant, meta.TenantServices[0].TenantId)
	assert.Equal(t, testSVCName, meta.TenantServices[0].Name)

	blockQuerier := sw.createBlockFromMeta(meta, meta.TenantServices[0])

	q := blockQuerier.Queriers()
	err = q.Open(context.Background())
	assert.NoError(t, err)

	res, err := q[0].SelectMergeByStacktraces(context.Background(), &ingestv1.SelectProfilesRequest{
		LabelSelector: fmt.Sprintf("{%s=\"%s\"}", model.LabelNameServiceName, testSVCName),
		Type:          cpuProfileType,
		Start:         0,
		End:           time.Now().UnixMilli(),
	}, 100)
	assert.NoError(t, err)
	collapsed := bytes.NewBuffer(nil)
	res.WriteCollapsed(collapsed)
	assert.Equal(t, string(collapsed.String()), "bar;foo 1\n")
}

func getMetadataDLQ(sw sw) []*metastorev1.BlockMeta {
	objects := sw.bucket.Objects()
	dlqFiles := []string{}
	for s := range objects {
		if strings.HasPrefix(s, pathDLQ) {
			dlqFiles = append(dlqFiles, s)
		}
	}
	slices.Sort(dlqFiles)
	var metas []*metastorev1.BlockMeta
	for _, s := range dlqFiles {
		var meta = new(metastorev1.BlockMeta)
		err := meta.UnmarshalVT(objects[s])
		assert.NoError(sw.t, err)
		metas = append(metas, meta)
	}
	return metas
	//get, err := sw.bucket.Get(context.Background(), fmt.Sprintf("%s/%d/%s/%s/%s", pathSegments, testShard, pathAnon, testProfile1.UUID, pathBlock))
	//if err != nil {
	//  return nil, err
	//}
	//blob, err := io.ReadAll(get)
	//if err != nil {
	//  return nil, err
	//}
	//var meta metastorev1.BlockMeta
	//err = meta.UnmarshalVT(blob)
	//return &meta, err
}

type sw struct {
	*segmentsWriter
	bucket    *memory.InMemBucket
	client    *metastoreClient
	phlarectx context.Context
	t         *testing.T
}

func newTestSegmentWriter(t *testing.T) sw {
	l := testutil.NewLogger(t)
	phlarectx := phlarecontext.WithLogger(context.Background(), l)
	reg := prometheus.NewRegistry()
	phlarectx = phlarecontext.WithRegistry(phlarectx, reg)
	cfg := phlaredb.Config{
		DataPath: t.TempDir(),
	}
	bucket := memory.NewInMemBucket()
	client := new(metastoreClient)
	res := newSegmentWriter(phlarectx,
		l,
		newSegmentMetrics(reg),
		cfg,
		bucket,
		1*time.Second,
		client,
	)
	return sw{
		t:              t,
		phlarectx:      phlarectx,
		segmentsWriter: res,
		bucket:         bucket,
		client:         client,
	}
}

func (sw *sw) createBlockFromMeta(meta *metastorev1.BlockMeta, ts *metastorev1.TenantService) *phlaredb.BlockQuerier {
	dir := sw.t.TempDir()
	blockid, err := ulid.New(uint64(meta.MaxTime), rand.Reader)
	require.NoError(sw.t, err)
	blockDir := filepath.Join(dir, blockid.String())
	sw.t.Logf("block dir: %s", blockDir)
	os.Mkdir(blockDir, 0755)
	blockBucket, err := filesystem.NewBucket(dir)
	blobReader, err := sw.bucket.Get(context.Background(), fmt.Sprintf("%s/%d/%s/%s/%s", pathSegments, testShard, pathAnon, meta.Id, pathBlock))
	require.NoError(sw.t, err)
	blob, err := io.ReadAll(blobReader)
	require.NoError(sw.t, err)

	//block.MetaFilename
	profiles := blob[ts.TableOfContents[0]:ts.TableOfContents[1]]
	tsdb := blob[ts.TableOfContents[1]:ts.TableOfContents[2]]
	symbols := blob[ts.TableOfContents[2] : ts.TableOfContents[0]+ts.Size]

	err = os.WriteFile(filepath.Join(blockDir, "profiles.parquet"), profiles, 0644)
	assert.NoError(sw.t, err)
	err = os.WriteFile(filepath.Join(blockDir, "index.tsdb"), tsdb, 0644)
	assert.NoError(sw.t, err)
	err = os.WriteFile(filepath.Join(blockDir, "symbols.symdb"), symbols, 0644)
	assert.NoError(sw.t, err)

	blockMeta := block.Meta{
		ULID:    blockid,
		MinTime: model2.TimeFromUnix(meta.MinTime),
		MaxTime: model2.TimeFromUnix(meta.MaxTime),
		Files: []block.File{
			{
				RelPath:   "profiles.parquet",
				SizeBytes: uint64(len(profiles)),
			},
			{
				RelPath:   "index.tsdb",
				SizeBytes: uint64(len(tsdb)),
			},
			{
				RelPath:   "symbols.symdb",
				SizeBytes: uint64(len(symbols)),
			},
		},
		Version: block.MetaVersion3,
	}
	blockMetaJson, err := json.Marshal(&blockMeta)
	assert.NoError(sw.t, err)
	err = os.WriteFile(filepath.Join(blockDir, block.MetaFilename), blockMetaJson, 0644)
	assert.NoError(sw.t, err)

	blockQuerier := phlaredb.NewBlockQuerier(sw.phlarectx, blockBucket)

	err = blockQuerier.Sync(context.Background())
	require.NoError(sw.t, err)

	return blockQuerier
}
