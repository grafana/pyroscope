package server

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
)

type queryExemplarsParams struct {
	input    storage.QueryExemplarsInput
	maxNodes int
	format   string
}

type queryExemplarsResponse struct {
	flamebearer.FlamebearerProfile
	Metadata queryExemplarsMetadataResponse `json:"metadata"`
}

type queryExemplarsMetadataResponse struct {
	flamebearer.FlamebearerMetadataV1
	AppName   string `json:"appName"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
	Query     string `json:"query"`
	MaxNodes  int    `json:"maxNodes"`
}

func (h ExemplarsHandler) QueryExemplars(w http.ResponseWriter, r *http.Request) {
	var p queryExemplarsParams
	if err := h.queryExemplarsParamsFromRequest(r, &p); err != nil {
		h.HTTPUtils.WriteInvalidParameterError(r, w, err)
		return
	}

	out, err := h.ExemplarsQuerier.QueryExemplars(r.Context(), p.input)
	if err != nil {
		h.HTTPUtils.WriteInternalServerError(r, w, err, "failed to retrieve data")
		return
	}

	flame := flamebearer.NewProfile(flamebearer.ProfileConfig{
		MaxNodes:  p.maxNodes,
		Metadata:  out.Metadata,
		Tree:      out.Tree,
		Heatmap:   h.HeatmapBuilder.BuildFromSketch(out.HeatmapSketch),
		Telemetry: out.Telemetry,
	})

	md := queryExemplarsMetadataResponse{
		FlamebearerMetadataV1: flame.Metadata,
		AppName:               p.input.Query.AppName,
		StartTime:             p.input.StartTime.Unix(),
		EndTime:               p.input.EndTime.Unix(),
		Query:                 p.input.Query.String(),
		MaxNodes:              p.maxNodes,
	}

	h.StatsReceiver.StatsInc("exemplars-query")
	h.HTTPUtils.WriteResponseJSON(r, w, queryExemplarsResponse{
		FlamebearerProfile: flame,
		Metadata:           md,
	})
}

func (h ExemplarsHandler) queryExemplarsParamsFromRequest(r *http.Request, p *queryExemplarsParams) (err error) {
	v := r.URL.Query()
	if p.input.Query, err = flameql.ParseQuery(v.Get("query")); err != nil {
		return fmt.Errorf("query: %w", err)
	}

	p.input.StartTime = parseTimeFallback(v.Get("startTime"), v.Get("from"))
	p.input.EndTime = parseTimeFallback(v.Get("endTime"), v.Get("until"))

	p.input.HeatmapParams.StartTime = p.input.StartTime
	p.input.HeatmapParams.EndTime = p.input.EndTime
	if p.input.HeatmapParams.MinValue, err = parseNumber(v.Get("minValue"), false); err != nil {
		return fmt.Errorf("can't parse minValue: %w", err)
	}
	if p.input.HeatmapParams.MaxValue, err = parseNumber(v.Get("maxValue"), true); err != nil {
		return fmt.Errorf("can't parse maxValue: %w", err)
	}
	if heatmapTimeBuckets := v.Get("heatmapTimeBuckets"); heatmapTimeBuckets != "" {
		if p.input.HeatmapParams.TimeBuckets, err = strconv.ParseInt(heatmapTimeBuckets, 10, 64); err != nil {
			return fmt.Errorf("can't parse heatmapTimeBuckets: %w", err)
		}
	}
	if heatmapValueBuckets := v.Get("heatmapValueBuckets"); heatmapValueBuckets != "" {
		if p.input.HeatmapParams.ValueBuckets, err = strconv.ParseInt(heatmapValueBuckets, 10, 64); err != nil {
			return fmt.Errorf("can't parse heatmapValueBuckets: %w", err)
		}
	}

	p.input.ExemplarsSelection.StartTime = parseTime(v.Get("selectionStartTime"))
	p.input.ExemplarsSelection.EndTime = parseTime(v.Get("selectionEndTime"))
	if p.input.ExemplarsSelection.MinValue, err = parseNumber(v.Get("selectionMinValue"), false); err != nil {
		return fmt.Errorf("can't parse selectionMinValue: %w", err)
	}
	if p.input.ExemplarsSelection.MaxValue, err = parseNumber(v.Get("selectionMaxValue"), true); err != nil {
		return fmt.Errorf("can't parse selectionMaxValue: %w", err)
	}

	if p.input.HeatmapParams.TimeBuckets == 0 && p.input.HeatmapParams.ValueBuckets == 0 {
		p.input.StartTime = p.input.ExemplarsSelection.StartTime
		p.input.EndTime = p.input.ExemplarsSelection.EndTime
	}

	p.maxNodes = h.MaxNodesDefault
	if newMaxNodes, ok := MaxNodesFromContext(r.Context()); ok {
		p.maxNodes = newMaxNodes
	}
	var x int
	if x, err = strconv.Atoi(v.Get("max-nodes")); err == nil && x != 0 {
		p.maxNodes = x
	}
	if x, err = strconv.Atoi(v.Get("maxNodes")); err == nil && x != 0 {
		p.maxNodes = x
	}

	return nil
}
