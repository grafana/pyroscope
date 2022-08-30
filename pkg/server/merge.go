package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/history"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

type MergeHandler struct {
	log             *logrus.Logger
	storage         storage.ExemplarsMerger
	dir             http.FileSystem
	stats           StatsReceiver
	maxNodesDefault int
	httpUtils       httputils.Utils
	historyMgr      history.Manager
}

type mergeRequest struct {
	QueryID   history.QueryID
	AppName   string   `json:"appName"`
	StartTime string   `json:"startTime"`
	EndTime   string   `json:"endTime"`
	Profiles  []string `json:"profiles"`
	MaxNodes  int      `json:"maxNodes"`

	// For consistency with render handler: `startTime` and `endTime` take precedence.
	From  string `json:"from"`
	Until string `json:"until"`
}

type mergeMetadata struct {
	AppName        string `json:"appName"`
	StartTime      string `json:"startTime"`
	EndTime        string `json:"endTime"`
	ProfilesLength int    `json:"profilesLength"`
}

type mergeResponse struct {
	flamebearer.FlamebearerProfile
	MergeMetadata *mergeMetadata  `json:"mergeMetadata"`
	QueryID       history.QueryID `json:"queryID"`
}

func (ctrl *Controller) mergeHandler() http.HandlerFunc {
	return NewMergeHandler(ctrl.log, ctrl.storage, ctrl.dir, ctrl, ctrl.config.MaxNodesRender, ctrl.httpUtils, ctrl.historyMgr).ServeHTTP
}

//revive:disable:argument-limit TODO(petethepig): we will refactor this later
func NewMergeHandler(
	l *logrus.Logger,
	s storage.ExemplarsMerger,
	dir http.FileSystem,
	stats StatsReceiver,
	maxNodesDefault int,
	httpUtils httputils.Utils,
	historyMgr history.Manager,
) *MergeHandler {
	return &MergeHandler{
		log:             l,
		storage:         s,
		dir:             dir,
		stats:           stats,
		maxNodesDefault: maxNodesDefault,
		httpUtils:       httpUtils,
		historyMgr:      historyMgr,
	}
}

func (mh *MergeHandler) mergeRequestFromQueryID(w http.ResponseWriter, r *http.Request, qid string) *mergeRequest {
	var req mergeRequest
	if qid != "" {
		res, err := mh.historyMgr.Get(r.Context(), history.QueryID(qid))
		if err != nil {
			mh.httpUtils.WriteInvalidParameterError(r, w, fmt.Errorf("error getting query: %v", err))
			return nil
		}
		if res == nil {
			mh.httpUtils.WriteInvalidParameterError(r, w, fmt.Errorf("queryID \"%v\" not found", qid))
			return nil
		}
		req.QueryID = history.QueryID(qid)
		req.AppName = res.AppName
		req.StartTime = strconv.Itoa(int(res.StartTime.Unix()))
		req.EndTime = strconv.Itoa(int(res.EndTime.Unix()))
		req.Profiles = res.Profiles
		// TODO: handle separately
		// req.MaxNodes = res.MaxNodes
	}
	return &req
}

func (mh *MergeHandler) mergeRequestFromJSONBody(w http.ResponseWriter, r *http.Request) *mergeRequest {
	var req mergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		mh.httpUtils.WriteInvalidParameterError(r, w, err)
		return nil
	}

	if req.AppName == "" {
		mh.httpUtils.WriteInvalidParameterError(r, w, fmt.Errorf("application name required"))
		return nil
	}
	if len(req.Profiles) == 0 {
		mh.httpUtils.WriteInvalidParameterError(r, w, fmt.Errorf("at least one profile ID must be specified"))
		return nil
	}
	return &req
}

func (mh *MergeHandler) mergeRequest(w http.ResponseWriter, r *http.Request) *mergeRequest {
	qid := r.URL.Query().Get("queryID")
	if qid != "" {
		return mh.mergeRequestFromQueryID(w, r, qid)
	}
	return mh.mergeRequestFromJSONBody(w, r)
}

func (mh *MergeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req := mh.mergeRequest(w, r)
	if req == nil {
		return
	}

	maxNodes := mh.maxNodesDefault
	if req.MaxNodes > 0 {
		maxNodes = req.MaxNodes
	}

	input := mergeExemplarsInputFromMergeRequest(req)
	out, err := mh.storage.MergeExemplars(r.Context(), input)
	if err != nil {
		mh.httpUtils.WriteInternalServerError(r, w, err, "failed to retrieve data")
		return
	}

	queryID := req.QueryID
	if queryID == "" {
		e := &history.Entry{
			Type:      history.EntryTypeMerge, //EntryType
			Timestamp: time.Now(),             //time.Time

			AppName:   input.AppName,
			Profiles:  input.ProfileIDs, //[]string
			StartTime: input.StartTime,
			EndTime:   input.EndTime,
		}
		e.PopulateFromRequest(r)
		var err error
		queryID, err = mh.historyMgr.Add(r.Context(), e)
		if err != nil {
			mh.httpUtils.WriteInternalServerError(r, w, err, "failed to save query")
			return
		}
	}

	flame := out.Tree.FlamebearerStruct(maxNodes)
	resp := mergeResponse{
		QueryID: queryID,
		MergeMetadata: &mergeMetadata{
			AppName:        req.AppName,
			StartTime:      req.StartTime,
			EndTime:        req.EndTime,
			ProfilesLength: len(req.Profiles),
		},
		FlamebearerProfile: flamebearer.FlamebearerProfile{
			Version: 1,
			FlamebearerProfileV1: flamebearer.FlamebearerProfileV1{
				Flamebearer: flamebearer.FlamebearerV1{
					Names:    flame.Names,
					Levels:   flame.Levels,
					NumTicks: flame.NumTicks,
					MaxSelf:  flame.MaxSelf,
				},
				Metadata: flamebearer.FlamebearerMetadataV1{
					Format:     string(tree.FormatSingle),
					SpyName:    out.SpyName,
					SampleRate: out.SampleRate,
					Units:      out.Units,
				},
			},
		},
	}

	mh.stats.StatsInc("merge")
	mh.httpUtils.WriteResponseJSON(r, w, resp)
}

func mergeExemplarsInputFromMergeRequest(req *mergeRequest) storage.MergeExemplarsInput {
	return storage.MergeExemplarsInput{
		AppName:    req.AppName,
		StartTime:  pickTime(req.StartTime, req.From),
		EndTime:    pickTime(req.EndTime, req.Until),
		ProfileIDs: req.Profiles,
	}
}

func pickTime(primary, fallback string) time.Time {
	if primary != "" {
		return attime.Parse(primary)
	}
	if fallback != "" {
		return attime.Parse(fallback)
	}
	return time.Time{}
}
