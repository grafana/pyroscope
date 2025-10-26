package v2

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/operations"
)

type blockQuery struct {
	From string
	To   string
	View string

	parsedFrom time.Time
	parsedTo   time.Time
}

func readQuery(r *http.Request) *blockQuery {
	queryFrom := r.URL.Query().Get("queryFrom")
	if queryFrom == "" {
		queryFrom = "now-24h"
	}
	parsedFrom, _ := operations.ParseTime(queryFrom)
	queryTo := r.URL.Query().Get("queryTo")
	if queryTo == "" {
		queryTo = "now"
	}
	parsedTo, _ := operations.ParseTime(queryTo)
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "table"
	}
	return &blockQuery{
		From:       queryFrom,
		To:         queryTo,
		View:       view,
		parsedFrom: parsedFrom,
		parsedTo:   parsedTo,
	}
}

type blockDetails struct {
	ID                string
	MinTime           string
	MaxTime           string
	Duration          int
	FormattedDuration string
	Shard             uint32
	CompactionLevel   uint32
	Size              string
	Datasets          []datasetDetails
	BlockTenant       string // Empty for multi-tenant blocks (compaction level 0)
}

type labelPair struct {
	Key   string
	Value string
}

type labelSet struct {
	Pairs []labelPair
}

type datasetDetails struct {
	Tenant             string
	Name               string
	MinTime            string
	MaxTime            string
	Size               string
	ProfilesSize       string
	IndexSize          string
	SymbolsSize        string
	ProfilesPercentage float64
	IndexPercentage    float64
	SymbolsPercentage  float64
	LabelSets          []labelSet
}

type blockGroup struct {
	MinTime                 time.Time
	FormattedMinTime        string
	Blocks                  []*blockDetails
	MinTimeAge              string
	MaxBlockDurationMinutes int
}

type blockListResult struct {
	BlockGroups          []*blockGroup
	MaxBlocksPerGroup    int
	GroupDurationMinutes int
}

// Sorts a slice of block groups by MinTime in descending order.
func sortBlockGroupsByMinTimeDec(bg []*blockGroup) {
	slices.SortFunc(bg, func(a, b *blockGroup) int {
		return b.MinTime.Compare(a.MinTime)
	})
}

// Sorts a slice of block details by MinTime in descending order.
func sortBlockDetailsByMinTimeDec(bd []*blockDetails) {
	slices.SortFunc(bd, func(a, b *blockDetails) int {
		return strings.Compare(b.MinTime, a.MinTime)
	})
}

type profileInfo struct {
	RowNumber   int
	Timestamp   string
	SeriesIndex uint32
	ProfileType string
	SampleCount int
}

type treeNode struct {
	Name           string
	Value          uint64
	Percent        float64
	Location       string // File path and line number (e.g., "pkg/util/logger.go:L91")
	FormattedValue string // Formatted value with unit (e.g., "1.5 MB", "250 ms")
	Children       []*treeNode
}

type profileCallTreePageContent struct {
	User        string
	BlockID     string
	Shard       uint32
	BlockTenant string
	Dataset     *datasetDetails
	Timestamp   string
	ProfileInfo *profileMetadata
	Tree        *treeNode
	Now         string
}

type profileMetadata struct {
	Labels      []labelPair
	SampleCount int
	Unit        string
	ProfileType string
}

type tsdbIndexInfo struct {
	From           string
	Through        string
	Checksum       uint32
	Series         []seriesInfo
	Symbols        []string
	LabelValueSets []labelValueSet
}

type labelValueSet struct {
	LabelName   string
	LabelValues []string
}

type seriesInfo struct {
	SeriesIndex uint32
	SeriesRef   uint64
	Labels      []labelPair
}

type datasetIndexPageContent struct {
	User        string
	BlockID     string
	Shard       uint32
	BlockTenant string
	Dataset     *datasetDetails
	TSDBIndex   *tsdbIndexInfo
	Now         string
}

type symbolsInfo struct {
	Strings        []symbolEntry
	TotalStrings   int
	Functions      []functionEntry
	TotalFunctions int
	Locations      []locationEntry
	TotalLocations int
	Mappings       []mappingEntry
	TotalMappings  int
}

type symbolEntry struct {
	Index  int
	Symbol string
}

type functionEntry struct {
	Index      int
	ID         uint64
	Name       string
	SystemName string
	Filename   string
	StartLine  uint32
}

type locationLine struct {
	FunctionName string
	Line         int64
}

type locationEntry struct {
	Index     int
	ID        uint64
	Address   uint64
	MappingID uint32
	Lines     []locationLine
}

type mappingEntry struct {
	Index       int
	ID          uint64
	MemoryStart uint64
	MemoryLimit uint64
	FileOffset  uint64
	Filename    string
	BuildID     string
}

type datasetSymbolsPageContent struct {
	User        string
	BlockID     string
	Shard       uint32
	BlockTenant string
	Dataset     *datasetDetails
	Symbols     *symbolsInfo
	Page        int
	PageSize    int
	TotalPages  int
	HasPrevPage bool
	HasNextPage bool
	Tab         string
	Now         string
}

const emptyDatasetPlaceholder = "_empty"

type datasetRequest struct {
	TenantID    string
	BlockID     string
	BlockTenant string
	DatasetName string
	Shard       uint32
}

func parseDatasetRequest(r *http.Request) (*datasetRequest, error) {
	vars := mux.Vars(r)

	tenantID := vars["tenant"]
	if tenantID == "" {
		return nil, errors.New("No tenant id provided")
	}

	blockID := vars["block"]
	if blockID == "" {
		return nil, errors.New("No block id provided")
	}

	datasetName := r.URL.Query().Get("dataset")
	if datasetName == "" {
		return nil, errors.New("No dataset name provided")
	}
	if datasetName == emptyDatasetPlaceholder {
		datasetName = ""
	}

	shardStr := r.URL.Query().Get("shard")
	if shardStr == "" {
		return nil, errors.New("No shard provided")
	}
	var shard uint32
	if _, err := fmt.Sscanf(shardStr, "%d", &shard); err != nil {
		return nil, errors.Wrap(err, "invalid shard parameter")
	}

	blockTenant := r.URL.Query().Get("block_tenant")

	return &datasetRequest{
		TenantID:    tenantID,
		BlockID:     blockID,
		BlockTenant: blockTenant,
		DatasetName: datasetName,
		Shard:       shard,
	}, nil
}

func (h *Handlers) getDatasetMetadata(ctx context.Context, req *datasetRequest) (*metastorev1.BlockMeta, *metastorev1.Dataset, error) {
	metadataResp, err := h.MetastoreClient.GetBlockMetadata(ctx, &metastorev1.GetBlockMetadataRequest{
		Blocks: &metastorev1.BlockList{
			Tenant: req.BlockTenant,
			Shard:  req.Shard,
			Blocks: []string{req.BlockID},
		},
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get block metadata")
	}

	if len(metadataResp.Blocks) == 0 {
		return nil, nil, errors.New("Block not found")
	}

	blockMeta := metadataResp.Blocks[0]

	var foundDataset *metastorev1.Dataset
	for _, ds := range blockMeta.Datasets {
		dsName := blockMeta.StringTable[ds.Name]
		if dsName == req.DatasetName {
			foundDataset = ds
			break
		}
	}

	if foundDataset == nil {
		return nil, nil, errors.New("Dataset not found")
	}

	return blockMeta, foundDataset, nil
}
