package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
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
	httpUtils       httputils.Utils
}

func (ctrl *Controller) mergeHandler() http.HandlerFunc {
	return NewMergeHandler(ctrl.log, ctrl.storage, ctrl.dir, ctrl, ctrl.config.MaxNodesRender, ctrl.httpUtils).ServeHTTP
}

//revive:disable:argument-limit TODO(petethepig): we will refactor this later
func NewMergeHandler(l *logrus.Logger, s storage.Merger, dir http.FileSystem, stats StatsReceiver, maxNodesDefault int, httpUtils httputils.Utils) *MergeHandler {
	return &MergeHandler{
		log:             l,
		storage:         s,
		dir:             dir,
		stats:           stats,
		maxNodesDefault: maxNodesDefault,
		httpUtils:       httpUtils,
	}
}

func (mh *MergeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req mergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		mh.httpUtils.WriteInvalidParameterError(w, err)
		return
	}

	if req.AppName == "" {
		mh.httpUtils.WriteInvalidParameterError(w, fmt.Errorf("application name required"))
		return
	}
	if len(req.Profiles) == 0 {
		mh.httpUtils.WriteInvalidParameterError(w, fmt.Errorf("at least one profile ID must be specified"))
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
		mh.httpUtils.WriteInternalServerError(w, err, "failed to retrieve data")
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
	mh.httpUtils.WriteResponseJSON(w, resp)
}
