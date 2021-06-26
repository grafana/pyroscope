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

	gOut, err := ctrl.storage.Get(&storage.GetInput{
		StartTime: startTime,
		EndTime:   endTime,
		Key:       storageKey,
	})
	ctrl.statsInc("render")
	if err != nil {
		panic(err) // TODO: handle
	}

	// TODO: handle properly
	if gOut == nil {
		gOut = &storage.GetOutput{
			Tree: tree.New(),
		}
	}

	maxNodes := ctrl.config.MaxNodesRender
	if mn, err := strconv.Atoi(q.Get("max-nodes")); err == nil && mn > 0 {
		maxNodes = mn
	}

	switch q.Get("format") {
	case "json":
		w.Header().Set("Content-Type", "application/json")

		fs := gOut.Tree.FlamebearerStruct(maxNodes)
		// TODO remove this duplication? We're already adding this to metadata
		fs.SpyName = gOut.SpyName
		fs.SampleRate = gOut.SampleRate
		fs.Units = gOut.Units
		res := map[string]interface{}{
			"timeline":    gOut.Timeline,
			"flamebearer": fs,
			"metadata": map[string]interface{}{
				"spyName":    gOut.SpyName,
				"sampleRate": gOut.SampleRate,
				"units":      gOut.Units,
			},
		}

		encoder := json.NewEncoder(w)
		encoder.Encode(res)
		return
	default:
		// TODO: add handling for other cases
		w.WriteHeader(422)
	}
}
