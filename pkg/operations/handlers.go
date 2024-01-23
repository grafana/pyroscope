package operations

import (
	"context"
	"math"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"

	"github.com/grafana/pyroscope/pkg/objstore"
	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/bucket"
	"github.com/grafana/pyroscope/pkg/phlaredb/bucketindex"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

type Handlers struct {
	Bucket           objstore.Bucket
	Logger           log.Logger
	MaxBlockDuration time.Duration
}

func (h *Handlers) CreateIndexHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		users, _ := bucket.ListUsers(r.Context(), h.Bucket)
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
		if tenantId == "" {
			httputil.Error(w, errors.New("No tenant id provided"))
			return
		}
		index, err := bucketindex.ReadIndex(r.Context(), h.Bucket, tenantId, nil, h.Logger)
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
			SelectedBlocks: h.filterAndGroupBlocks(index, query),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}

func (h *Handlers) filterAndGroupBlocks(index *bucketindex.Index, query *blockQuery) *blockListResult {
	queryFrom := model.TimeFromUnix(query.parsedFrom.UnixMilli() / 1000)
	queryTo := model.TimeFromUnix(query.parsedTo.UnixMilli() / 1000)
	blockGroupMap := make(map[time.Time]*blockGroup)
	blockGroups := make([]*blockGroup, 0)

	deletedBlocks := make(map[ulid.ULID]int64)
	if !query.IncludeDeleted {
		for _, deletionMark := range index.BlockDeletionMarks {
			deletedBlocks[deletionMark.ID] = deletionMark.DeletionTime
		}
	}

	for _, blk := range index.Blocks {
		if _, deleted := deletedBlocks[blk.ID]; !deleted && blk.Within(queryFrom, queryTo) {
			minTime := blk.MinTime.Time().UTC()
			truncatedMinTime := minTime.Truncate(h.MaxBlockDuration)
			blkGroup, ok := blockGroupMap[truncatedMinTime]
			if !ok {
				blkGroup = &blockGroup{
					MinTime:                 truncatedMinTime,
					FormattedMinTime:        truncatedMinTime.Format(time.RFC3339),
					Blocks:                  make([]*blockDetails, 0),
					MinTimeAge:              humanize.RelTime(blk.MinTime.Time(), time.Now(), "ago", ""),
					MaxBlockDurationMinutes: int(math.Round(blk.MaxTime.Sub(blk.MinTime).Minutes())),
				}
				blockGroups = append(blockGroups, blkGroup)
			}
			blockDetails := &blockDetails{
				ID:                blk.ID.String(),
				MinTime:           minTime.Format(time.RFC3339),
				MaxTime:           blk.MaxTime.Time().UTC().Format(time.RFC3339),
				Duration:          int(math.Round(blk.MaxTime.Sub(blk.MinTime).Minutes())),
				FormattedDuration: blk.MaxTime.Sub(blk.MinTime).Round(time.Minute).String(),
				UploadedAt:        blk.GetUploadedAt().UTC().Format(time.RFC3339),
				CompactionLevel:   blk.CompactionLevel,
				CompactorShardID:  blk.CompactorShardID,
			}
			blkGroup.Blocks = append(blkGroup.Blocks, blockDetails)
			if blockDetails.Duration > blkGroup.MaxBlockDurationMinutes {
				blkGroup.MaxBlockDurationMinutes = blockDetails.Duration
			}
			blockGroupMap[truncatedMinTime] = blkGroup
		}
	}

	sortBlockGroupsByMinTimeDec(blockGroups)

	maxBlocksPerGroup := 0
	maxBlockGroupDuration := 0
	for _, blockGroup := range blockGroups {
		sortBlockDetailsByMinTimeDec(blockGroup.Blocks)
		if len(blockGroup.Blocks) > maxBlocksPerGroup {
			maxBlocksPerGroup = len(blockGroup.Blocks)
		}
		if blockGroup.MaxBlockDurationMinutes > maxBlockGroupDuration {
			maxBlockGroupDuration = blockGroup.MaxBlockDurationMinutes
		}
	}

	return &blockListResult{
		BlockGroups:          blockGroups,
		MaxBlocksPerGroup:    maxBlocksPerGroup,
		GroupDurationMinutes: maxBlockGroupDuration,
	}
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

		blockDetails := getBlockDetails(r.Context(), bId, fetcher)
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
		Duration:        int(math.Round(meta.MaxTime.Sub(meta.MinTime).Minutes())),
		CompactionLevel: meta.Compaction.Level,
		Size:            humanize.Bytes(blockSize),
		Stats:           meta.Stats,
		Labels:          meta.Labels,
	}
}
