package v2

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

type MetastoreClient interface {
	QueryMetadata(ctx context.Context, req *metastorev1.QueryMetadataRequest, opts ...grpc.CallOption) (*metastorev1.QueryMetadataResponse, error)
	GetTenants(ctx context.Context, req *metastorev1.GetTenantsRequest, opts ...grpc.CallOption) (*metastorev1.GetTenantsResponse, error)
	GetBlockMetadata(ctx context.Context, req *metastorev1.GetBlockMetadataRequest, opts ...grpc.CallOption) (*metastorev1.GetBlockMetadataResponse, error)
}

type Handlers struct {
	MetastoreClient MetastoreClient
	Logger          log.Logger
}

func (h *Handlers) CreateIndexHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := h.MetastoreClient.GetTenants(r.Context(), &metastorev1.GetTenantsRequest{})
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to get tenants"))
			return
		}

		slices.SortFunc(resp.TenantIds, func(a, b string) int {
			return strings.Compare(a, b)
		})

		err = pageTemplates.indexTemplate.Execute(w, indexPageContent{
			Users: resp.TenantIds,
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

		query := readQuery(r)
		startTimeMs := query.parsedFrom.UnixMilli()
		endTimeMs := query.parsedTo.UnixMilli()

		metadataResp, err := h.MetastoreClient.QueryMetadata(r.Context(), &metastorev1.QueryMetadataRequest{
			TenantId:  []string{tenantId},
			Query:     "{}",
			StartTime: startTimeMs,
			EndTime:   endTimeMs,
		})
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to query metadata for blocks"))
			return
		}

		err = pageTemplates.blocksTemplate.Execute(w, blockListPageContent{
			User:           tenantId,
			Query:          query,
			Now:            time.Now().UTC().Format(time.RFC3339),
			SelectedBlocks: h.groupBlocks(metadataResp.Blocks),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}

func (h *Handlers) groupBlocks(blocks []*metastorev1.BlockMeta) *blockListResult {
	blockGroupMap := make(map[time.Time]*blockGroup)
	blockGroups := make([]*blockGroup, 0)

	for _, blk := range blocks {
		minTime := msToTime(blk.MinTime).UTC()
		maxTime := msToTime(blk.MaxTime).UTC()
		truncatedMinTime := minTime.Truncate(time.Hour)

		blkGroup, ok := blockGroupMap[truncatedMinTime]
		if !ok {
			blkGroup = &blockGroup{
				MinTime:                 truncatedMinTime,
				FormattedMinTime:        truncatedMinTime.Format(time.RFC3339),
				Blocks:                  make([]*blockDetails, 0),
				MinTimeAge:              humanize.RelTime(minTime, time.Now(), "ago", ""),
				MaxBlockDurationMinutes: durationInMinutes(minTime, maxTime),
			}
			blockGroups = append(blockGroups, blkGroup)
			blockGroupMap[truncatedMinTime] = blkGroup
		}

		duration := durationInMinutes(minTime, maxTime)

		blockDetails := &blockDetails{
			ID:                blk.Id,
			MinTime:           minTime.Format(time.RFC3339),
			MaxTime:           maxTime.Format(time.RFC3339),
			Duration:          duration,
			FormattedDuration: formatDuration(duration),
			Shard:             blk.Shard,
			CompactionLevel:   blk.CompactionLevel,
			Size:              humanize.Bytes(blk.Size),
			BlockTenant:       blk.StringTable[blk.Tenant],
		}

		blkGroup.Blocks = append(blkGroup.Blocks, blockDetails)
		if duration > blkGroup.MaxBlockDurationMinutes {
			blkGroup.MaxBlockDurationMinutes = duration
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
		shardStr := r.URL.Query().Get("shard")
		if shardStr == "" {
			httputil.Error(w, errors.New("No shard provided"))
			return
		}
		var shard uint32
		if _, err := fmt.Sscanf(shardStr, "%d", &shard); err != nil {
			httputil.Error(w, errors.Wrap(err, "invalid shard parameter"))
			return
		}

		blockTenant := r.URL.Query().Get("block_tenant")

		metadataResp, err := h.MetastoreClient.GetBlockMetadata(r.Context(), &metastorev1.GetBlockMetadataRequest{
			Blocks: &metastorev1.BlockList{
				Tenant: blockTenant,
				Shard:  shard,
				Blocks: []string{blockId},
			},
		})
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to get block metadata"))
			return
		}
		if len(metadataResp.Blocks) == 0 {
			httputil.Error(w, errors.New("Block not found"))
			return
		}

		blockMeta := metadataResp.Blocks[0]

		blockDetails := h.convertBlockMeta(blockMeta)
		err = pageTemplates.blockDetailsTemplate.Execute(w, blockDetailsPageContent{
			User:        tenantId,
			Block:       blockDetails,
			Shard:       shard,
			BlockTenant: blockTenant,
			Now:         time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}

func (h *Handlers) convertDataset(ds *metastorev1.Dataset, stringTable []string) datasetDetails {
	tenant := stringTable[ds.Tenant]
	datasetName := stringTable[ds.Name]

	var labelSets []labelSet
	pairs := metadata.LabelPairs(ds.Labels)
	for pairs.Next() {
		p := pairs.At()
		var currentSet labelSet
		for len(p) > 0 {
			if len(p) >= 2 {
				key := stringTable[p[0]]
				val := stringTable[p[1]]
				currentSet.Pairs = append(currentSet.Pairs, labelPair{Key: key, Value: val})
				p = p[2:]
			} else {
				break
			}
		}
		if len(currentSet.Pairs) > 0 {
			labelSets = append(labelSets, currentSet)
		}
	}

	var profilesSize, indexSize, symbolsSize uint64
	if len(ds.TableOfContents) >= 3 {
		profilesSize = ds.TableOfContents[1] - ds.TableOfContents[0]
		indexSize = ds.TableOfContents[2] - ds.TableOfContents[1]
		symbolsSize = (ds.TableOfContents[0] + ds.Size) - ds.TableOfContents[2]
	}

	return datasetDetails{
		Tenant:       tenant,
		Name:         datasetName,
		MinTime:      msToTime(ds.MinTime).UTC().Format(time.RFC3339),
		MaxTime:      msToTime(ds.MaxTime).UTC().Format(time.RFC3339),
		Size:         humanize.Bytes(ds.Size),
		ProfilesSize: humanize.Bytes(profilesSize),
		IndexSize:    humanize.Bytes(indexSize),
		SymbolsSize:  humanize.Bytes(symbolsSize),
		LabelSets:    labelSets,
	}
}

func (h *Handlers) convertBlockMeta(meta *metastorev1.BlockMeta) *blockDetails {
	minTime := msToTime(meta.MinTime).UTC()
	maxTime := msToTime(meta.MaxTime).UTC()
	duration := durationInMinutes(minTime, maxTime)

	datasets := make([]datasetDetails, 0, len(meta.Datasets))
	for _, ds := range meta.Datasets {
		datasets = append(datasets, h.convertDataset(ds, meta.StringTable))
	}

	return &blockDetails{
		ID:                meta.Id,
		MinTime:           minTime.Format(time.RFC3339),
		MaxTime:           maxTime.Format(time.RFC3339),
		Duration:          duration,
		FormattedDuration: formatDuration(duration),
		Shard:             meta.Shard,
		CompactionLevel:   meta.CompactionLevel,
		Size:              humanize.Bytes(meta.Size),
		Datasets:          datasets,
	}
}

func (h *Handlers) CreateDatasetDetailsHandler() func(http.ResponseWriter, *http.Request) {
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
		datasetName := r.URL.Query().Get("dataset")
		if datasetName == "" {
			httputil.Error(w, errors.New("No dataset name provided"))
			return
		}
		// Handle special case for empty dataset name
		if datasetName == "_empty" {
			datasetName = ""
		}
		shardStr := r.URL.Query().Get("shard")
		if shardStr == "" {
			httputil.Error(w, errors.New("No shard provided"))
			return
		}
		var shard uint32
		if _, err := fmt.Sscanf(shardStr, "%d", &shard); err != nil {
			httputil.Error(w, errors.Wrap(err, "invalid shard parameter"))
			return
		}

		blockTenant := r.URL.Query().Get("block_tenant")

		metadataResp, err := h.MetastoreClient.GetBlockMetadata(r.Context(), &metastorev1.GetBlockMetadataRequest{
			Blocks: &metastorev1.BlockList{
				Tenant: blockTenant,
				Shard:  shard,
				Blocks: []string{blockId},
			},
		})
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to get block metadata"))
			return
		}

		if len(metadataResp.Blocks) == 0 {
			httputil.Error(w, errors.New("Block not found"))
			return
		}

		blockMeta := metadataResp.Blocks[0]

		var foundDataset *metastorev1.Dataset
		for _, ds := range blockMeta.Datasets {
			dsName := blockMeta.StringTable[ds.Name]
			if dsName == datasetName {
				foundDataset = ds
				break
			}
		}

		if foundDataset == nil {
			httputil.Error(w, errors.New("Dataset not found"))
			return
		}

		dataset := h.convertDataset(foundDataset, blockMeta.StringTable)

		err = pageTemplates.datasetDetailsTemplate.Execute(w, datasetDetailsPageContent{
			User:        tenantId,
			BlockID:     blockId,
			Shard:       shard,
			BlockTenant: blockTenant,
			Dataset:     &dataset,
			Now:         time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}
