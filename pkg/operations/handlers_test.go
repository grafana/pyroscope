package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/grafana/dskit/concurrency"
	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/objstore/testutil"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/bucketindex"
)

func TestHandlers_CreateBlockDetailsHandler(t *testing.T) {
	const userID = "user-1"

	ctx := context.Background()

	bkt, _ := testutil.NewFilesystemBucket(t, ctx, t.TempDir())
	now := time.Now()
	logs := &concurrency.SyncBuffer{}
	logger := log.NewLogfmtLogger(logs)

	// Create a bucket index.
	blk := &bucketindex.Block{ID: ulid.MustNew(1, nil), MinTime: model.Now().Add(-2 * time.Hour), MaxTime: model.Now()}
	blkMeta := block.Meta{ULID: blk.ID, Compaction: block.BlockMetaCompaction{Level: 3}, Version: block.MetaVersion3}
	metaJson, _ := json.Marshal(blkMeta)
	require.NoError(t, bkt.Upload(ctx, fmt.Sprintf("user-1/phlaredb/%s/meta.json", blk.ID.String()), bytes.NewReader(metaJson)))

	require.NoError(t, bucketindex.WriteIndex(ctx, bkt, userID, nil, &bucketindex.Index{
		Version:            bucketindex.IndexVersion3,
		Blocks:             bucketindex.Blocks{blk},
		BlockDeletionMarks: bucketindex.BlockDeletionMarks{},
		UpdatedAt:          now.Unix(),
	}))

	handlers := Handlers{
		Bucket: bkt,
		Logger: logger,
	}

	m := mux.NewRouter()
	m.HandleFunc("/tenants/{tenant}/blocks/{block}", handlers.CreateBlockDetailsHandler())

	req := httptest.NewRequest("GET", "/tenants/user-1/blocks/"+blk.ID.String(), nil)
	resp := httptest.NewRecorder()

	m.ServeHTTP(resp, req)

	require.Equal(t, 200, resp.Code)
	require.True(t, strings.Contains(resp.Body.String(), blk.ID.String()))
	require.True(t, strings.Contains(resp.Body.String(), "<td>3</td>"))
}

func TestHandlers_CreateBlocksHandler(t *testing.T) {
	const userID = "user-1"

	ctx := context.Background()

	bkt, _ := testutil.NewFilesystemBucket(t, ctx, t.TempDir())
	now := time.Now()
	logs := &concurrency.SyncBuffer{}
	logger := log.NewLogfmtLogger(logs)

	// Create a bucket index.
	block1 := &bucketindex.Block{ID: ulid.MustNew(1, nil), MinTime: model.Now().Add(-2 * time.Hour), MaxTime: model.Now()}
	block2 := &bucketindex.Block{ID: ulid.MustNew(2, nil), MinTime: model.Now().Add(-4 * time.Hour), MaxTime: model.Now()}
	block3 := &bucketindex.Block{ID: ulid.MustNew(3, nil), MinTime: model.Now().Add(-4 * time.Hour), MaxTime: model.Now()}
	block4 := &bucketindex.Block{ID: ulid.MustNew(4, nil), MinTime: model.Now().Add(-12 * time.Hour), MaxTime: model.Now().Add(-10 * time.Hour)}

	require.NoError(t, bucketindex.WriteIndex(ctx, bkt, userID, nil, &bucketindex.Index{
		Version:            bucketindex.IndexVersion1,
		Blocks:             bucketindex.Blocks{block1, block2, block3, block4},
		BlockDeletionMarks: bucketindex.BlockDeletionMarks{},
		UpdatedAt:          now.Unix(),
	}))

	handlers := Handlers{
		Bucket: bkt,
		Logger: logger,
	}

	m := mux.NewRouter()
	m.HandleFunc("/tenants/{tenant}/blocks", handlers.CreateBlocksHandler())

	req := httptest.NewRequest("GET", "/tenants/user-1/blocks?queryFrom=now-4h", nil)
	resp := httptest.NewRecorder()

	m.ServeHTTP(resp, req)

	require.Equal(t, 200, resp.Code)
	require.True(t, strings.Contains(resp.Body.String(), block1.ID.String()))
	require.True(t, strings.Contains(resp.Body.String(), block2.ID.String()))
	require.True(t, strings.Contains(resp.Body.String(), block3.ID.String()))
	require.False(t, strings.Contains(resp.Body.String(), block4.ID.String())) // outside the now-4h window
}

func TestHandlers_CreateIndexHandler(t *testing.T) {
	const userID = "user-1"

	ctx := context.Background()

	bkt, _ := testutil.NewFilesystemBucket(t, ctx, t.TempDir())
	logs := &concurrency.SyncBuffer{}
	logger := log.NewLogfmtLogger(logs)

	// Create a bucket index.
	require.NoError(t, bucketindex.WriteIndex(ctx, bkt, userID, nil, &bucketindex.Index{
		Version:            bucketindex.IndexVersion3,
		Blocks:             bucketindex.Blocks{},
		BlockDeletionMarks: bucketindex.BlockDeletionMarks{},
	}))

	handlers := Handlers{
		Bucket: bkt,
		Logger: logger,
	}

	h := handlers.CreateIndexHandler()

	req := httptest.NewRequest("GET", "/", nil)
	resp := httptest.NewRecorder()

	h(resp, req)

	require.Equal(t, 200, resp.Code)
	require.True(t, strings.Contains(resp.Body.String(), "user-1"))
}

func Test_filterAndGroupBlocks(t *testing.T) {
	block1 := &bucketindex.Block{ID: ulid.MustNew(1, nil), MinTime: model.Now().Add(-2 * time.Hour), MaxTime: model.Now().Add(-1 * time.Hour)}
	block2 := &bucketindex.Block{ID: ulid.MustNew(2, nil), MinTime: model.Now().Add(-4 * time.Hour), MaxTime: model.Now().Add(-3 * time.Hour)}
	block3 := &bucketindex.Block{ID: ulid.MustNew(3, nil), MinTime: model.Now().Add(-4*time.Hour + time.Minute), MaxTime: model.Now().Add(-3 * time.Hour)}
	block4 := &bucketindex.Block{ID: ulid.MustNew(4, nil), MinTime: model.Now().Add(-12 * time.Hour), MaxTime: model.Now().Add(-10 * time.Hour)}
	h := &Handlers{MaxBlockDuration: time.Hour}

	type args struct {
		index *bucketindex.Index
		query *blockQuery
	}
	tests := []struct {
		name string
		args args
		want *blockListResult
	}{
		{
			name: "empty index",
			args: args{
				index: &bucketindex.Index{
					Version:            bucketindex.IndexVersion3,
					Blocks:             bucketindex.Blocks{},
					BlockDeletionMarks: bucketindex.BlockDeletionMarks{},
				}, query: &blockQuery{}},
			want: &blockListResult{
				BlockGroups:          []*blockGroup{},
				GroupDurationMinutes: 0,
			},
		},
		{
			name: "index with blocks that can be filtered and grouped",
			args: args{
				index: &bucketindex.Index{
					Version:            bucketindex.IndexVersion3,
					Blocks:             bucketindex.Blocks{block1, block2, block3, block4},
					BlockDeletionMarks: bucketindex.BlockDeletionMarks{&bucketindex.BlockDeletionMark{ID: block1.ID}},
				},
				query: &blockQuery{
					parsedFrom: time.Now().Add(-6 * time.Hour),
					parsedTo:   time.Now(),
				}},
			want: &blockListResult{
				// block 1 is not included because it is marked as deleted
				// block 4 is not included because it is outside the query window
				BlockGroups: []*blockGroup{
					{
						MinTime:          block2.MinTime.Time().Truncate(time.Hour).UTC(),
						FormattedMinTime: block2.MinTime.Time().Truncate(time.Hour).UTC().Format(time.RFC3339),
						Blocks: []*blockDetails{
							{
								ID:                block3.ID.String(), // block 3 is newer so it goes first
								MinTime:           block3.MinTime.Time().UTC().Format(time.RFC3339),
								MaxTime:           block3.MaxTime.Time().UTC().Format(time.RFC3339),
								Duration:          59,
								FormattedDuration: "59m0s",
								UploadedAt:        time.UnixMilli(0).UTC().Format(time.RFC3339),
							},
							{
								ID:                block2.ID.String(),
								MinTime:           block2.MinTime.Time().UTC().Format(time.RFC3339),
								MaxTime:           block2.MaxTime.Time().UTC().Format(time.RFC3339),
								Duration:          60,
								FormattedDuration: "1h0m0s",
								UploadedAt:        time.UnixMilli(0).UTC().Format(time.RFC3339),
							},
						},
						MinTimeAge:              "4 hours ago",
						MaxBlockDurationMinutes: 60,
					},
				},
				GroupDurationMinutes: 60,
				MaxBlocksPerGroup:    2,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, h.filterAndGroupBlocks(tt.args.index, tt.args.query), "filterAndGroupBlocks(%v, %v)", tt.args.index, tt.args.query)
		})
	}
}
