package server

import (
	"encoding/json"
	"fmt"
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

	resultTree, tl, err := ctrl.s.Get(startTime, endTime, storageKey)
	if err != nil {
		panic(err) // TODO: handle
	}

	if resultTree == nil {
		resultTree = tree.New()
	}

	if q.Get("format") == "frontend" {
		w.Header().Set("Content-Type", "text/plain+pyroscope")
	} else if q.Get("format") == "svg" {
		w.Header().Set("Content-Type", "image/svg+xml")
	}

	filename := q.Get("download-filename")
	log.WithField("filename", filename).Debug("filename")
	if filename != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	}
	w.WriteHeader(200)

	if q.Get("format") == "frontend" {
		encoder := json.NewEncoder(w)
		encoder.Encode(tl)
	}

	minVal := uint64(0)
	log.Debug("minVal", minVal)

	width := 1200
	if newVal, err := strconv.Atoi(q.Get("width")); err == nil && newVal > 0 {
		width = newVal
	}

	resultTree.SVG(w, 1024, width, globalMultiplier)
}
