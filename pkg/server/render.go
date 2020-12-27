package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

type samplesEntry struct {
	Ts      time.Time `json:"ts"`
	Samples uint16    `json:"samples"`
}

func (ctrl *Controller) renderHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	startTime := attime.Parse(q.Get("from"))
	endTime := attime.Parse(q.Get("until"))
	var err error
	storageKey, err := storage.ParseKey(q.Get("name"))
	if err != nil {
		panic(err) // TODO: handle
	}

	resultTree, tl, err := ctrl.s.Get(startTime, endTime, storageKey)
	if err != nil {
		panic(err) // TODO: handle
	}

	if resultTree == nil {
		resultTree = tree.New()
	}

	maxNodes := ctrl.cfg.Server.MaxNodesRender
	if mn, err := strconv.Atoi(q.Get("max-nodes")); err == nil && mn > 0 {
		maxNodes = mn
	}

	switch q.Get("format") {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)

		res := map[string]interface{}{
			"timeline":    tl,
			"flamebearer": resultTree.FlamebearerStruct(maxNodes),
		}
		encoder := json.NewEncoder(w)
		encoder.Encode(res)
		return
	default:
		// TODO: add handling for other cases
		w.WriteHeader(422)
	}
}
