package operations

import (
	"net/http"
	"strings"
	"time"

	"golang.org/x/exp/slices"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

type blockQuery struct {
	From           string
	To             string
	IncludeDeleted bool
	View           string

	parsedFrom time.Time
	parsedTo   time.Time
}

func readQuery(r *http.Request) *blockQuery {
	queryFrom := r.URL.Query().Get("queryFrom")
	if queryFrom == "" {
		queryFrom = "now-24h"
	}
	parsedFrom, _ := ParseTime(queryFrom)
	queryTo := r.URL.Query().Get("queryTo")
	if queryTo == "" {
		queryTo = "now"
	}
	parsedTo, _ := ParseTime(queryTo)
	includeDeleted := r.URL.Query().Get("includeDeleted")
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "table"
	}
	return &blockQuery{
		From:           queryFrom,
		To:             queryTo,
		IncludeDeleted: includeDeleted != "",
		View:           view,
		parsedFrom:     parsedFrom,
		parsedTo:       parsedTo,
	}
}

type blockDetails struct {
	ID                string
	MinTime           string
	MaxTime           string
	Duration          int
	FormattedDuration string
	UploadedAt        string
	CompactorShardID  string
	CompactionLevel   int
	Size              string
	Stats             block.BlockStats
	Labels            map[string]string
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
