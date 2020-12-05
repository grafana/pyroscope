package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/petethepig/pyroscope/pkg/storage"
	"github.com/petethepig/pyroscope/pkg/storage/tree"
	"github.com/petethepig/pyroscope/pkg/util/attime"
	log "github.com/sirupsen/logrus"
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

	samplesEntries := []*samplesEntry{}

	resultTree, err := ctrl.s.Get(startTime, endTime, storageKey)
	if err != nil {
		panic(err) // TODO: handle
	}

	if resultTree == nil {
		resultTree = tree.New()
	}

	w.WriteHeader(200)
	if q.Get("format") == "frontend" {
		w.Header().Set("Content-Type", "text/plain+pyroscope")
		encoder := json.NewEncoder(w)
		encoder.Encode(samplesEntries)
	} else if q.Get("format") == "svg" {
		w.Header().Set("Content-Type", "image/svg+xml")
	}

	minVal := uint64(0)
	log.Debug("minVal", minVal)

	width := 1200
	if newVal, err := strconv.Atoi(q.Get("width")); err == nil && newVal > 0 {
		width = newVal
	}

	resultTree.SVG(w, 1024, width)
}
