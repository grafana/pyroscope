package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/sirupsen/logrus"
)

type MergeHandler struct {
	log             *logrus.Logger
	storage         storage.Merger
	dir             http.FileSystem
	stats           StatsReceiver
	maxNodesDefault int
}

func (ctrl *Controller) mergeHandler() http.Handler {
	return NewMergeHandler(ctrl.log, ctrl.storage, ctrl.dir, ctrl, ctrl.config.MaxNodesRender)
}

func NewMergeHandler(l *logrus.Logger, s storage.Merger, dir http.FileSystem, stats StatsReceiver, maxNodesDefault int) *MergeHandler {
	return &MergeHandler{
		log:             l,
		storage:         s,
		dir:             dir,
		stats:           stats,
		maxNodesDefault: maxNodesDefault,
	}
}

func (mh *MergeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req mergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		mh.writeInvalidParameterError(w, err)
		return
	}

	if req.AppName == "" {
		mh.writeInvalidParameterError(w, fmt.Errorf("application name required"))
		return
	}
	if len(req.Profiles) == 0 {
		mh.writeInvalidParameterError(w, fmt.Errorf("at least one profile ID must be specified"))
		return
	}
	maxNodes := mh.maxNodesDefault
	if req.MaxNodes > 0 {
		maxNodes = req.MaxNodes
	}

	out, err := mh.storage.MergeProfiles(r.Context(), storage.MergeProfilesInput{
		AppName:  req.AppName,
		Profiles: req.Profiles,
	})
	if err != nil {
		mh.writeInternalServerError(w, err, "failed to retrieve data")
		return
	}

	flame := out.Tree.FlamebearerStruct(maxNodes)
	resp := mergeResponse{
		FlamebearerProfile: flamebearer.FlamebearerProfile{
			Version: 1,
			FlamebearerProfileV1: flamebearer.FlamebearerProfileV1{
				Flamebearer: flamebearer.FlamebearerV1{
					Names:    flame.Names,
					Levels:   flame.Levels,
					NumTicks: flame.NumTicks,
					MaxSelf:  flame.MaxSelf,
				},
				// Hardcoded values for Go.
				Metadata: flamebearer.FlamebearerMetadataV1{
					Format:     string(tree.FormatSingle),
					SpyName:    "unknown",
					SampleRate: 100,
					Units:      "samples",
				},
			},
		},
	}

	mh.stats.StatsInc("merge")
	mh.writeResponseJSON(w, resp)
}

// TODO: remove this

func (mh *MergeHandler) writeResponseJSON(w http.ResponseWriter, res interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		mh.writeJSONEncodeError(w, err)
	}
}

func (*MergeHandler) writeResponseFile(w http.ResponseWriter, filename string, content []byte) {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%v", filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(content)
	w.(http.Flusher).Flush()
}

func (mh *MergeHandler) writeError(w http.ResponseWriter, code int, err error, msg string) {
	WriteError(mh.log, w, code, err, msg)
}

func (mh *MergeHandler) writeInvalidMethodError(w http.ResponseWriter) {
	WriteErrorMessage(mh.log, w, http.StatusMethodNotAllowed, "method not allowed")
}

func (mh *MergeHandler) writeInvalidParameterError(w http.ResponseWriter, err error) {
	mh.writeError(w, http.StatusBadRequest, err, "invalid parameter")
}

func (mh *MergeHandler) writeInternalServerError(w http.ResponseWriter, err error, msg string) {
	mh.writeError(w, http.StatusInternalServerError, err, msg)
}

func (mh *MergeHandler) writeJSONEncodeError(w http.ResponseWriter, err error) {
	mh.writeInternalServerError(w, err, "encoding response body")
}

func (mh *MergeHandler) writeErrorMessage(w http.ResponseWriter, code int, msg string) {
	WriteErrorMessage(mh.log, w, code, msg)
}
