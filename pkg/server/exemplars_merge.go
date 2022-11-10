package server

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/history"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/heatmap"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

type mergeExemplarsRequest struct {
	QueryID  history.QueryID
	AppName  string   `json:"appName"`
	Profiles []string `json:"profiles"`
	MaxNodes int      `json:"maxNodes"`

	StartTime           string `json:"startTime"`
	EndTime             string `json:"endTime"`
	MinValue            uint64 `json:"minValue"`
	MaxValue            uint64 `json:"maxValue"`
	HeatmapTimeBuckets  int64  `json:"heatmapTimeBuckets"`
	HeatmapValueBuckets int64  `json:"heatmapValueBuckets"`

	SelectionStartTime string `json:"selectionStartTime"`
	SelectionEndTime   string `json:"selectionEndTime"`
	SelectionMinValue  uint64 `json:"selectionMinValue"`
	SelectionMaxValue  uint64 `json:"selectionMaxValue"`

	// For consistency with render handler: `startTime` and `endTime` take precedence.
	From  string `json:"from"`
	Until string `json:"until"`
}

type mergeExemplarsResponse struct {
	flamebearer.FlamebearerProfile
	Metadata *mergeExemplarsMetadata `json:"mergeMetadata"`
	QueryID  history.QueryID         `json:"queryID"`
}

type mergeExemplarsMetadata struct {
	flamebearer.FlamebearerMetadataV1
	AppName        string `json:"appName"`
	StartTime      string `json:"startTime"`
	EndTime        string `json:"endTime"`
	MaxNodes       int    `json:"maxNodes"`
	ProfilesLength int    `json:"profilesLength"`
}

func (h ExemplarsHandler) MergeExemplars(w http.ResponseWriter, r *http.Request) {
	req := h.mergeExemplarsRequest(w, r)
	if req == nil {
		return
	}

	maxNodes := h.MaxNodesDefault
	if newMaxNodes, ok := MaxNodesFromContext(r.Context()); ok {
		maxNodes = newMaxNodes
	}
	if req.MaxNodes != 0 {
		maxNodes = req.MaxNodes
	}

	input := mergeExemplarsInputFromMergeExemplarsRequest(req)
	out, err := h.ExemplarsMerger.MergeExemplars(r.Context(), input)
	if err != nil {
		h.HTTPUtils.WriteInternalServerError(r, w, err, "failed to retrieve data")
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
		queryID, err = h.HistoryManager.Add(r.Context(), e)
		if err != nil {
			h.HTTPUtils.WriteInternalServerError(r, w, err, "failed to save query")
			return
		}
	}

	flame := flamebearer.NewProfile(flamebearer.ProfileConfig{
		MaxNodes:  maxNodes,
		Metadata:  out.Metadata,
		Tree:      out.Tree,
		Heatmap:   h.HeatmapBuilder.BuildFromSketch(out.HeatmapSketch),
		Telemetry: out.Telemetry,
	})

	md := &mergeExemplarsMetadata{
		FlamebearerMetadataV1: flame.Metadata,
		AppName:               input.AppName,
		StartTime:             strconv.Itoa(int(input.StartTime.Unix())),
		EndTime:               strconv.Itoa(int(input.EndTime.Unix())),
		MaxNodes:              req.MaxNodes,
		ProfilesLength:        len(req.Profiles),
	}

	h.StatsReceiver.StatsInc("merge")
	h.HTTPUtils.WriteResponseJSON(r, w, mergeExemplarsResponse{
		QueryID:            queryID,
		FlamebearerProfile: flame,
		Metadata:           md,
	})
}

func (h *ExemplarsHandler) mergeExemplarsRequestFromQueryID(w http.ResponseWriter, r *http.Request, qid string) *mergeExemplarsRequest {
	var req mergeExemplarsRequest
	if qid == "" {
		res, err := h.HistoryManager.Get(r.Context(), history.QueryID(qid))
		if err != nil {
			h.HTTPUtils.WriteInvalidParameterError(r, w, fmt.Errorf("error getting query: %v", err))
			return nil
		}
		if res == nil {
			h.HTTPUtils.WriteInvalidParameterError(r, w, fmt.Errorf("queryID \"%v\" not found", qid))
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

func (h *ExemplarsHandler) mergeExemplarsRequestFromJSONBody(w http.ResponseWriter, r *http.Request) *mergeExemplarsRequest {
	var req mergeExemplarsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.HTTPUtils.WriteInvalidParameterError(r, w, err)
		return nil
	}

	if req.AppName == "" {
		h.HTTPUtils.WriteInvalidParameterError(r, w, fmt.Errorf("application name required"))
		return nil
	}
	if len(req.Profiles) == 0 {
		h.HTTPUtils.WriteInvalidParameterError(r, w, fmt.Errorf("at least one profile ID must be specified"))
		return nil
	}
	return &req
}

func (h *ExemplarsHandler) mergeExemplarsRequest(w http.ResponseWriter, r *http.Request) *mergeExemplarsRequest {
	qid := r.URL.Query().Get("queryID")
	if qid != "" {
		return h.mergeExemplarsRequestFromQueryID(w, r, qid)
	}
	return h.mergeExemplarsRequestFromJSONBody(w, r)
}

func mergeExemplarsInputFromMergeExemplarsRequest(req *mergeExemplarsRequest) storage.MergeExemplarsInput {
	startTime := parseTimeFallback(req.StartTime, req.From)
	endTime := parseTimeFallback(req.EndTime, req.Until)
	return storage.MergeExemplarsInput{
		AppName:    req.AppName,
		ProfileIDs: req.Profiles,
		StartTime:  startTime,
		EndTime:    endTime,
		ExemplarsSelection: storage.ExemplarsSelection{
			StartTime: parseTime(req.SelectionStartTime),
			EndTime:   parseTime(req.SelectionEndTime),
			MinValue:  req.MinValue,
			MaxValue:  req.MaxValue,
		},
		HeatmapParams: heatmap.HeatmapParams{
			StartTime:    startTime,
			EndTime:      endTime,
			MinValue:     req.MinValue,
			MaxValue:     req.MaxValue,
			TimeBuckets:  req.HeatmapTimeBuckets,
			ValueBuckets: req.HeatmapValueBuckets,
		},
	}
}

func parseTimeFallback(primary, fallback string) time.Time {
	if primary != "" {
		return attime.Parse(primary)
	}
	if fallback != "" {
		return attime.Parse(fallback)
	}
	return time.Unix(0, 0)
}

func parseTime(t string) time.Time {
	if t == "" {
		return time.Unix(0, 0)
	}
	return attime.Parse(t)
}

func parseNumber(n string, ceil bool) (uint64, error) {
	if n == "" {
		return 0, nil
	}
	x, err := strconv.ParseUint(n, 10, 64)
	if err == nil {
		return x, nil
	}
	f, err := strconv.ParseFloat(n, 64)
	if err == nil {
		if ceil {
			return uint64(math.Ceil(f)), nil
		}
		return uint64(f), nil
	}
	return 0, fmt.Errorf("invalid value: expected uint or float: %q", n)
}
