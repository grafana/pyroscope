package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/petethepig/pyroscope/pkg/storage"
	"github.com/petethepig/pyroscope/pkg/storage/tree"
	"github.com/petethepig/pyroscope/pkg/timing"
	"github.com/petethepig/pyroscope/pkg/util/attime"
	log "github.com/sirupsen/logrus"
)

type samplesEntry struct {
	Ts      time.Time `json:"ts"`
	Samples uint16    `json:"samples"`
}

func (ctrl *Controller) renderHandler(w http.ResponseWriter, r *http.Request) {
	timer := timing.New()
	timer.Measure("render", func() {
		q := r.URL.Query()
		startTime := attime.Parse(q.Get("from"))
		endTime := attime.Parse(q.Get("until"))
		var err error
		storageKey, err := storage.ParseKey(q.Get("name"))
		if err != nil {
			panic(err) // TODO: handle
		}

		log.Debug("storageKey", storageKey.Normalized())
		samplesEntries := []*samplesEntry{}

		resultTrie, err := ctrl.s.Get(startTime, endTime, storageKey)
		if err != nil {
			panic(err) // TODO: handle
		}

		if resultTrie == nil {
			resultTrie = tree.New()
		}

		cb := func(tr *tree.Tree, w2 io.Writer) {
			minVal := uint64(0)
			log.Debug("minVal", minVal)

			width := 1200
			if newVal, err := strconv.Atoi(q.Get("width")); err == nil && newVal > 0 {
				width = newVal
			}

			tr.SVG(w2, 1024, width)
		}

		w.WriteHeader(200)
		if q.Get("format") == "frontend" {
			w.Header().Set("Content-Type", "text/plain+pyroscope")
			timer.Measure("callback", func() {
				encoder := json.NewEncoder(w)
				encoder.Encode(samplesEntries)
				cb(resultTrie, w)
			})
		} else if q.Get("format") == "svg" {
			w.Header().Set("Content-Type", "image/svg+xml")
			timer.Measure("callback", func() {
				cb(resultTrie, w)
			})
		} else {
			cb(resultTrie, w)
		}
	})
}
