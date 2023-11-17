package operations

import (
	"net/http"
	"time"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

type blockQuery struct {
	From           string `json:"from,omitempty"`
	To             string `json:"to,omitempty"`
	IncludeDeleted bool   `json:"includeDeleted,omitempty"`

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
	return &blockQuery{
		From:           queryFrom,
		To:             queryTo,
		IncludeDeleted: includeDeleted != "",
		parsedFrom:     parsedFrom,
		parsedTo:       parsedTo,
	}
}

type blockDetails struct {
	ID               string            `json:"id,omitempty"`
	MinTime          string            `json:"minTime,omitempty"`
	MaxTime          string            `json:"maxTime,omitempty"`
	Duration         string            `json:"duration,omitempty"`
	UploadedAt       string            `json:"uploadedAt,omitempty"`
	CompactorShardID string            `json:"compactorShardID,omitempty"`
	CompactionLevel  int               `json:"compactionLevel,omitempty"`
	Size             string            `json:"size,omitempty"`
	Stats            block.BlockStats  `json:"stats,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
}

type blockGroup struct {
	MinTime    string          `json:"minTime,omitempty"`
	Blocks     []*blockDetails `json:"blocks,omitempty"`
	MinTimeAge string          `json:"minTimeAge,omitempty"`
}
