package main

import (
	"context"
	"embed"
	_ "embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/grafana/dskit/server"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"golang.org/x/exp/slices"

	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	objstoreclient "github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/objstore/providers/gcs"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/bucket"
	"github.com/grafana/pyroscope/pkg/phlaredb/bucketindex"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

//go:embed tool/blocks/tool.blocks.index.gohtml
var indexPageHtml string

//go:embed tool/blocks/tool.blocks.list.gohtml
var blocksPageHtml string

//go:embed tool/blocks/tool.blocks.detail.gohtml
var blockDetailsPageHtml string

//go:embed tool/static
var staticFiles embed.FS

type blocksWebToolParams struct {
	httpListenPort  int
	objectStoreType string
	bucketName      string
}

func addBlocksWebToolParams(blocksWebToolCmd commander) *blocksWebToolParams {
	var (
		params = &blocksWebToolParams{}
	)
	blocksWebToolCmd.Flag("object-store-type", "The type of the object storage (e.g., gcs).").Default("gcs").StringVar(&params.objectStoreType)
	blocksWebToolCmd.Flag("bucket-name", "The name of the object storage bucket.").StringVar(&params.bucketName)
	blocksWebToolCmd.Flag("http-listen-port", "The port to run the HTTP server on.").Default("4201").IntVar(&params.httpListenPort)
	return params
}

type indexPageContent struct {
	Users []string
	Now   string
}

type blockListPageContent struct {
	User           string
	Index          *bucketindex.Index
	SelectedPeriod string
	SelectedBlocks []*blockGroup
	Query          *blockQuery
	Now            string
}

type blockDetailsPageContent struct {
	User  string
	Block *blockDetails
	Now   string
}

type blockQuery struct {
	From           string
	To             string
	IncludeDeleted bool

	parsedFrom time.Time
	parsedTo   time.Time
}

type blockGroup struct {
	MinTime    string
	Blocks     []*blockDetails
	MinTimeAge string
}

type blockDetails struct {
	ID               string
	MinTime          string
	MaxTime          string
	Duration         string
	UploadedAt       string
	CompactorShardID string
	CompactionLevel  int
	Size             string
	Stats            block.BlockStats
	Labels           map[string]string
}

type blocksWebTool struct {
	params *blocksWebToolParams
	server *server.Server
	bucket phlareobj.Bucket
}

func newBlocksWebTool(params *blocksWebToolParams) *blocksWebTool {
	ctx := context.Background()
	var (
		serverCfg = server.Config{
			HTTPListenPort: params.httpListenPort,
			Log:            logger,
		}
	)

	s, err := server.New(serverCfg)
	if err != nil {
		log.Fatal(err)
		return nil
	}
	s.HTTP.PathPrefix("/tool/static/").Handler(http.FileServer(http.FS(staticFiles)))

	b, err := initObjectStoreBucket(params)
	if err != nil {
		log.Fatal(err)
		return nil
	}

	tool := &blocksWebTool{
		params: params,
		server: s,
		bucket: b,
	}

	indexTemplate := template.New("index")
	template.Must(indexTemplate.Parse(indexPageHtml))

	s.HTTP.Path("/blocks/index").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		users, _ := bucket.ListUsers(ctx, b)
		err := indexTemplate.Execute(w, indexPageContent{
			Users: users,
			Now:   time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			httputil.Error(w, err)
		}
	})

	blocksTemplate := template.New("blocks")
	template.Must(blocksTemplate.Parse(blocksPageHtml))

	s.HTTP.Path("/blocks/list").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantId := r.URL.Query().Get("tenantId")
		index, err := bucketindex.ReadIndex(ctx, tool.bucket, tenantId, nil, logger)
		if err != nil {
			httputil.Error(w, err)
			return
		}
		query := readQuery(r)
		err = blocksTemplate.Execute(w, blockListPageContent{
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
	})

	blockDetailsTemplate := template.New("block-details")
	template.Must(blockDetailsTemplate.Parse(blockDetailsPageHtml))

	s.HTTP.Path("/blocks/details").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantId := r.URL.Query().Get("tenantId")
		if tenantId == "" {
			httputil.Error(w, errors.New("No tenant id provided"))
			return
		}
		blockId := r.URL.Query().Get("blockId")
		if blockId == "" {
			httputil.Error(w, errors.New("No block id provided"))
			return
		}
		bId, err := ulid.Parse(blockId)
		if err != nil {
			httputil.Error(w, err)
			return
		}

		prefixedBucket := phlareobj.NewPrefixedBucket(tool.bucket, path.Join(tenantId, "phlaredb/"))
		defer prefixedBucket.Close()

		fetcher, err := block.NewMetaFetcher(logger, 16, prefixedBucket, os.TempDir(), nil, nil)
		if err != nil {
			httputil.Error(w, err)
			return
		}

		blockDetails := getBlockDetails(ctx, bId, fetcher)
		if blockDetails != nil {
			err = blockDetailsTemplate.Execute(w, blockDetailsPageContent{
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
	})

	return tool
}

func initObjectStoreBucket(params *blocksWebToolParams) (phlareobj.Bucket, error) {
	objectStoreConfig := objstoreclient.Config{
		StoragePrefix: "",
		StorageBackendConfig: objstoreclient.StorageBackendConfig{
			Backend: params.objectStoreType,
			GCS: gcs.Config{
				BucketName: params.bucketName,
			},
		},
	}
	return objstoreclient.NewBucket(
		context.Background(),
		objectStoreConfig,
		"storage",
	)
}

func readQuery(r *http.Request) *blockQuery {
	queryFrom := r.URL.Query().Get("queryFrom")
	if queryFrom == "" {
		queryFrom = "now-24h"
	}
	parsedFrom, _ := parseTime(queryFrom)
	queryTo := r.URL.Query().Get("queryTo")
	if queryTo == "" {
		queryTo = "now"
	}
	parsedTo, _ := parseTime(queryTo)
	includeDeleted := r.URL.Query().Get("includeDeleted")
	return &blockQuery{
		From:           queryFrom,
		To:             queryTo,
		IncludeDeleted: includeDeleted != "",
		parsedFrom:     parsedFrom,
		parsedTo:       parsedTo,
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

func (t *blocksWebTool) run(ctx context.Context) error {
	out := output(ctx)

	fmt.Fprintf(out, "The blocks web tool is available at http://localhost:%d/blocks/index\n", t.params.httpListenPort)

	if err := t.server.Run(); err != nil {
		return err
	}

	return nil
}
