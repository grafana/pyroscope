// SPDX-License-Identifier: AGPL-3.0-only

package storegateway

import (
	_ "embed" // Used to embed html template
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gorilla/mux"
	"github.com/grafana/mimir/pkg/storage/tsdb"
	"github.com/grafana/mimir/pkg/util"

	"github.com/grafana/phlare/pkg/phlaredb/block"
)

//go:embed blocks.gohtml
var blocksPageHTML string
var blocksPageTemplate = template.Must(template.New("webpage").Parse(blocksPageHTML))

type blocksPageContents struct {
	Now             time.Time            `json:"now"`
	Tenant          string               `json:"tenant,omitempty"`
	RichMetas       []richMeta           `json:"metas"`
	FormattedBlocks []formattedBlockData `json:"-"`
	ShowDeleted     bool                 `json:"-"`
	ShowSources     bool                 `json:"-"`
	ShowParents     bool                 `json:"-"`
	SplitCount      int                  `json:"-"`
}

type formattedBlockData struct {
	ULID            string
	ULIDTime        string
	SplitID         *uint32
	MinTime         string
	MaxTime         string
	Duration        string
	DeletedTime     string
	CompactionLevel int
	BlockSize       string
	Labels          string
	Sources         []string
	Parents         []string
	Stats           block.BlockStats
}

type richMeta struct {
	*block.Meta
	// DeletedTime *int64  `json:"deletedTime,omitempty"`
	SplitID *uint32 `json:"splitId,omitempty"`
}

func (s *StoreGateway) BlocksHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	tenantID := vars["tenant"]
	if tenantID == "" {
		util.WriteTextResponse(w, "Tenant ID can't be empty")
		return
	}

	if err := req.ParseForm(); err != nil {
		util.WriteTextResponse(w, fmt.Sprintf("Can't parse form: %s", err))
		return
	}

	showDeleted := req.Form.Get("show_deleted") == "on"
	showSources := req.Form.Get("show_sources") == "on"
	showParents := req.Form.Get("show_parents") == "on"
	var splitCount int
	if sc := req.Form.Get("split_count"); sc != "" {
		splitCount, _ = strconv.Atoi(sc)
		if splitCount < 0 {
			splitCount = 0
		}
	}

	metasMap, err := block.ListBlocks(filepath.Join(s.gatewayCfg.BucketStoreConfig.SyncDir, tenantID), time.Time{})
	if err != nil {
		util.WriteTextResponse(w, fmt.Sprintf("Failed to read block metadata: %s", err))
		return
	}
	metas := block.SortBlocks(metasMap)

	formattedBlocks := make([]formattedBlockData, 0, len(metas))
	richMetas := make([]richMeta, 0, len(metas))

	for _, m := range metas {
		// if !showDeleted && !deletedTimes[m.ULID].IsZero() {
		// 	continue
		// }
		var parents []string
		for _, pb := range m.Compaction.Parents {
			parents = append(parents, pb.ULID.String())
		}
		var sources []string
		for _, pb := range m.Compaction.Sources {
			sources = append(parents, pb.String())
		}
		var blockSplitID *uint32
		if splitCount > 0 {
			bsc := tsdb.HashBlockID(m.ULID) % uint32(splitCount)
			blockSplitID = &bsc
		}

		var blockSize uint64
		for _, f := range m.Files {
			blockSize += f.SizeBytes
		}

		formattedBlocks = append(formattedBlocks, formattedBlockData{
			ULID:            m.ULID.String(),
			ULIDTime:        util.TimeFromMillis(int64(m.ULID.Time())).UTC().Format(time.RFC3339),
			SplitID:         blockSplitID,
			MinTime:         util.TimeFromMillis(int64(m.MinTime)).UTC().Format(time.RFC3339),
			MaxTime:         util.TimeFromMillis(int64(m.MaxTime)).UTC().Format(time.RFC3339),
			Duration:        util.TimeFromMillis(int64(m.MaxTime)).Sub(util.TimeFromMillis(int64(m.MinTime))).String(),
			CompactionLevel: m.Compaction.Level,
			BlockSize:       humanize.Bytes(blockSize),
			Stats:           m.Stats,
			Sources:         sources,
			Parents:         parents,
		})

		richMetas = append(richMetas, richMeta{
			Meta:    m,
			SplitID: blockSplitID,
		})
	}

	util.RenderHTTPResponse(w, blocksPageContents{
		Now:             time.Now(),
		Tenant:          tenantID,
		RichMetas:       richMetas,
		FormattedBlocks: formattedBlocks,

		SplitCount:  splitCount,
		ShowDeleted: showDeleted,
		ShowSources: showSources,
		ShowParents: showParents,
	}, blocksPageTemplate, req)
}

// func formatTimeIfNotZero(t time.Time, format string) string {
// 	if t.IsZero() {
// 		return ""
// 	}

// 	return t.Format(format)
// }
