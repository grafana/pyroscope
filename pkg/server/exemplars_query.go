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

	flame := flamebearer.NewProfileWithConfig(flamebearer.ProfileConfig{
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

	p.input.StartTime = pickTime(v.Get("startTime"), v.Get("from"))
	p.input.EndTime = pickTime(v.Get("endTime"), v.Get("until"))

	p.input.MinValue, _ = strconv.ParseUint(v.Get("minValue"), 10, 64)
	p.input.MaxValue, _ = strconv.ParseUint(v.Get("maxValue"), 10, 64)
	p.input.HeatmapParams = storage.HeatmapParams{
		StartTime: p.input.StartTime,
		EndTime:   p.input.EndTime,
	}

	p.input.HeatmapParams.TimeBuckets, _ = strconv.ParseInt(v.Get("heatmapTimeBuckets"), 10, 64)
	p.input.HeatmapParams.ValueBuckets, _ = strconv.ParseInt(v.Get("heatmapValueBuckets"), 10, 64)

	p.maxNodes = h.MaxNodesDefault
	var x int
	if x, err = strconv.Atoi(v.Get("max-nodes")); err == nil && x > 0 {
		p.maxNodes = x
	}
	if x, err = strconv.Atoi(v.Get("maxNodes")); err == nil && x > 0 {
		p.maxNodes = x
	}

	return nil
}
