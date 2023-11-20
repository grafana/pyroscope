package operations

import (
	"context"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"golang.org/x/exp/slices"

	"github.com/grafana/pyroscope/pkg/objstore"
	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/bucket"
	"github.com/grafana/pyroscope/pkg/phlaredb/bucketindex"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

type Handlers struct {
	Bucket  objstore.Bucket
	Logger  log.Logger
	Context context.Context
}

func (h *Handlers) CreateIndexHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		users, _ := bucket.ListUsers(h.Context, h.Bucket)
		err := pageTemplates.indexTemplate.Execute(w, indexPageContent{
			Users: users,
			Now:   time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			httputil.Error(w, err)
		}
	}
}

func (h *Handlers) CreateBlocksHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		tenantId := vars["tenant"]
		index, err := bucketindex.ReadIndex(h.Context, h.Bucket, tenantId, nil, h.Logger)
		if err != nil {
			httputil.Error(w, err)
			return
		}
		query := readQuery(r)
		err = pageTemplates.blocksTemplate.Execute(w, blockListPageContent{
			Index:          index,
			User:           tenantId,
			Query:          query,
			Now:            time.Now().UTC().Format(time.RFC3339),
			SelectedBlocks: filterAndGroupBlocks(index, query),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}

func filterAndGroupBlocks(index *bucketindex.Index, query *blockQuery) []*blockGroup {
	queryFrom := model.TimeFromUnix(query.parsedFrom.UnixMilli() / 1000)
	queryTo := model.TimeFromUnix(query.parsedTo.UnixMilli() / 1000)
	blockGroupMap := make(map[string]*blockGroup)
	blockGroups := make([]*blockGroup, 0)

	deletedBlocks := make(map[ulid.ULID]int64)
	if !query.IncludeDeleted {
		for _, deletionMark := range index.BlockDeletionMarks {
			deletedBlocks[deletionMark.ID] = deletionMark.DeletionTime
		}
	}

	for _, blk := range index.Blocks {
		if _, deleted := deletedBlocks[blk.ID]; !deleted && blk.Within(queryFrom, queryTo) {
			minTime := blk.MinTime.Time().UTC().Format(time.RFC3339)
			blkGroup, ok := blockGroupMap[minTime]
			if !ok {
				blkGroup = &blockGroup{
					MinTime:    minTime,
					Blocks:     make([]*blockDetails, 0),
					MinTimeAge: humanize.RelTime(blk.MinTime.Time(), time.Now(), "ago", ""),
				}
				blockGroups = append(blockGroups, blkGroup)
			}
			blkGroup.Blocks = append(blkGroup.Blocks, &blockDetails{
				ID:               blk.ID.String(),
				MinTime:          minTime,
				MaxTime:          blk.MaxTime.Time().UTC().Format(time.RFC3339),
				Duration:         blk.MaxTime.Sub(blk.MinTime).String(),
				UploadedAt:       blk.GetUploadedAt().UTC().Format(time.RFC3339),
				CompactionLevel:  blk.CompactionLevel,
				CompactorShardID: blk.CompactorShardID,
			})
			blockGroupMap[minTime] = blkGroup
		}
	}

	slices.SortFunc(blockGroups, func(a, b *blockGroup) bool {
		return a.MinTime > b.MinTime
	})

	return postProcessBlockGroups(blockGroups)
}

func postProcessBlockGroups(blockGroups []*blockGroup) []*blockGroup {
	for i := 0; i < len(blockGroups)-1; i += 1 {
		blockGroup := blockGroups[i]
		if !strings.Contains(blockGroup.MinTime, "0:00Z") {
			nextGroup := blockGroups[i+1]
			nextGroup.Blocks = append(nextGroup.Blocks, blockGroup.Blocks...)
			blockGroup.Blocks = make([]*blockDetails, 0)
		}
		slices.SortFunc(blockGroup.Blocks, func(a, b *blockDetails) bool {
			return a.MinTime > b.MinTime
		})
	}

	finalBlockGroups := make([]*blockGroup, 0)
	for _, blockGroup := range blockGroups {
		if len(blockGroup.Blocks) > 0 {
			finalBlockGroups = append(finalBlockGroups, blockGroup)
		}
	}
	return finalBlockGroups
}

func (h *Handlers) CreateBlockDetailsHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		tenantId := vars["tenant"]
		if tenantId == "" {
			httputil.Error(w, errors.New("No tenant id provided"))
			return
		}
		blockId := vars["block"]
		if blockId == "" {
			httputil.Error(w, errors.New("No block id provided"))
			return
		}
		bId, err := ulid.Parse(blockId)
		if err != nil {
			httputil.Error(w, err)
			return
		}

		prefixedBucket := phlareobj.NewPrefixedBucket(h.Bucket, path.Join(tenantId, "phlaredb/"))
		defer prefixedBucket.Close()

		fetcher, err := block.NewMetaFetcher(h.Logger, 1, prefixedBucket, os.TempDir(), nil, nil)
		if err != nil {
			httputil.Error(w, err)
			return
		}

		blockDetails := getBlockDetails(h.Context, bId, fetcher)
		if blockDetails != nil {
			err = pageTemplates.blockDetailsTemplate.Execute(w, blockDetailsPageContent{
				User:  tenantId,
				Block: blockDetails,
				Now:   time.Now().UTC().Format(time.RFC3339),
			})
			if err != nil {
				httputil.Error(w, err)
				return
			}
		} else {
			httputil.Error(w, errors.New("Could not find block"))
			return
		}
	}
}

func getBlockDetails(ctx context.Context, id ulid.ULID, fetcher *block.MetaFetcher) *blockDetails {
	meta, err := fetcher.LoadMeta(ctx, id)
	if err != nil {
		return nil
	}
	var blockSize uint64
	for _, f := range meta.Files {
		blockSize += f.SizeBytes
	}

	return &blockDetails{
		ID:              meta.ULID.String(),
		MinTime:         meta.MinTime.Time().UTC().Format(time.RFC3339),
		MaxTime:         meta.MaxTime.Time().UTC().Format(time.RFC3339),
		Duration:        meta.MaxTime.Sub(meta.MinTime).String(),
		CompactionLevel: meta.Compaction.Level,
		Size:            humanize.Bytes(blockSize),
		Stats:           meta.Stats,
		Labels:          meta.Labels,
	}
}
