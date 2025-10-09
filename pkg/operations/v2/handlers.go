package v2

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

type MetastoreClient interface {
	QueryMetadata(ctx context.Context, req *metastorev1.QueryMetadataRequest, opts ...grpc.CallOption) (*metastorev1.QueryMetadataResponse, error)
	GetTenants(ctx context.Context, req *metastorev1.GetTenantsRequest, opts ...grpc.CallOption) (*metastorev1.GetTenantsResponse, error)
	GetBlockMetadata(ctx context.Context, req *metastorev1.GetBlockMetadataRequest, opts ...grpc.CallOption) (*metastorev1.GetBlockMetadataResponse, error)
}

type Handlers struct {
	MetastoreClient  MetastoreClient
	Logger           log.Logger
	MaxBlockDuration time.Duration
}

func (h *Handlers) CreateIndexHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := h.MetastoreClient.GetTenants(r.Context(), &metastorev1.GetTenantsRequest{})
		if err != nil {
			httputil.Error(w, errors.Wrap(err, "failed to get tenants"))
			return
		}

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
			httputil.Error(w, errors.Wrap(err, "failed to query metadata"))
			return
		}

		err = pageTemplates.blocksTemplate.Execute(w, blockListPageContent{
			User:           tenantId,
			Query:          query,
			Now:            time.Now().UTC().Format(time.RFC3339),
			SelectedBlocks: h.groupBlocks(metadataResp.Blocks, query),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}

func (h *Handlers) groupBlocks(blocks []*metastorev1.BlockMeta, query *blockQuery) *blockListResult {
	blockGroupMap := make(map[time.Time]*blockGroup)
	blockGroups := make([]*blockGroup, 0)

	for _, blk := range blocks {
		minTime := msToTime(blk.MinTime).UTC()
		maxTime := msToTime(blk.MaxTime).UTC()
		truncatedMinTime := minTime.Truncate(h.MaxBlockDuration)

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

		// For multi-tenant blocks, Tenant field is 0 (empty in string table)
		// For single-tenant blocks, Tenant field points to the tenant string
		blockTenantStr := ""
		if blk.Tenant > 0 && int(blk.Tenant) < len(blk.StringTable) {
			blockTenantStr = blk.StringTable[blk.Tenant]
		}

		blockDetails := &blockDetails{
			ID:                blk.Id,
			MinTime:           minTime.Format(time.RFC3339),
			MaxTime:           maxTime.Format(time.RFC3339),
			Duration:          duration,
			FormattedDuration: formatDuration(duration),
			Shard:             blk.Shard,
			CompactionLevel:   blk.CompactionLevel,
			Size:              humanize.Bytes(blk.Size),
			BlockTenant:       blockTenantStr,
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

		// Get block_tenant from query parameter
		// For multi-tenant blocks (compaction level 0), this will be empty string
		// For single-tenant blocks (compaction level > 0), this will be the tenant ID
		blockTenant := r.URL.Query().Get("block_tenant")

		// Use GetBlockMetadata to retrieve the specific block
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
			User:  tenantId,
			Block: blockDetails,
			Now:   time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			httputil.Error(w, err)
			return
		}
	}
}

func (h *Handlers) convertBlockMeta(meta *metastorev1.BlockMeta) *blockDetails {
	minTime := msToTime(meta.MinTime).UTC()
	maxTime := msToTime(meta.MaxTime).UTC()
	duration := durationInMinutes(minTime, maxTime)

	// Resolve labels from string table
	labels := make(map[string]string)
	// TODO: Parse the labels field from datasets according to the format specified in types.proto
	// The labels are stored as a slice of int32 values referencing the string table
	// Format: len(2) | k1 | v1 | k2 | v2 | len(3) | k1 | v3 | k2 | v4 | k3 | v5

	// Parse datasets
	datasets := make([]datasetDetails, 0, len(meta.Datasets))
	for _, ds := range meta.Datasets {
		tenantName := ""
		if ds.Tenant >= 0 && int(ds.Tenant) < len(meta.StringTable) {
			tenantName = meta.StringTable[ds.Tenant]
		}
		datasetName := ""
		if ds.Name >= 0 && int(ds.Name) < len(meta.StringTable) {
			datasetName = meta.StringTable[ds.Name]
		}

		datasets = append(datasets, datasetDetails{
			Tenant:  tenantName,
			Name:    datasetName,
			MinTime: msToTime(ds.MinTime).UTC().Format(time.RFC3339),
			MaxTime: msToTime(ds.MaxTime).UTC().Format(time.RFC3339),
			Size:    humanize.Bytes(ds.Size),
		})
	}

	// TODO: BlockStats are not available in BlockMeta
	// We need to clarify how to get these stats in v2
	stats := block.BlockStats{
		NumSeries:   0, // Not available in v2 BlockMeta
		NumProfiles: 0, // Not available in v2 BlockMeta
		NumSamples:  0, // Not available in v2 BlockMeta
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
		Stats:             stats,
		Labels:            labels,
		Datasets:          datasets,
	}
}
