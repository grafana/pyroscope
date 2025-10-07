package v2

import (
	"net/http"
	"slices"
	"strings"
	"time"
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
	parsedFrom, _ := parseTime(queryFrom)
	queryTo := r.URL.Query().Get("queryTo")
	if queryTo == "" {
		queryTo = "now"
	}
	parsedTo, _ := parseTime(queryTo)
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
	Labels            map[string]string
	Datasets          []datasetDetails
	BlockTenant       string // Empty for multi-tenant blocks (compaction level 0)
}

type datasetDetails struct {
	Tenant       string
	Name         string
	MinTime      string
	MaxTime      string
	Size         string
	ProfilesSize string
	IndexSize    string
	SymbolsSize  string
	Labels       map[string]string
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
