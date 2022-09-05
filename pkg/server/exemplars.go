package server

import (
	"github.com/pyroscope-io/pyroscope/pkg/history"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

func (ctrl *Controller) exemplarsHandler() ExemplarsHandler {
	return ExemplarsHandler{
		StatsReceiver:    ctrl,
		HTTPUtils:        ctrl.httpUtils,
		HistoryManager:   ctrl.historyMgr,
		MaxNodesDefault:  ctrl.config.MaxNodesRender,
		ExemplarsGetter:  ctrl.storage,
		ExemplarsMerger:  ctrl.storage,
		ExemplarsQuerier: ctrl.storage,
		HeatmapBuilder:   NoopHeatmapBuilder{},
	}
}

type ExemplarsHandler struct {
	StatsReceiver  StatsReceiver
	HistoryManager history.Manager
	HTTPUtils      httputils.Utils

	MaxNodesDefault int

	ExemplarsGetter  storage.ExemplarsGetter
	ExemplarsMerger  storage.ExemplarsMerger
	ExemplarsQuerier storage.ExemplarsQuerier
	HeatmapBuilder   HeatmapBuilder
}

type HeatmapBuilder interface {
	BuildFromSketch(storage.HeatmapSketch) *storage.Heatmap
}

type NoopHeatmapBuilder struct{}

func (NoopHeatmapBuilder) BuildFromSketch(storage.HeatmapSketch) *storage.Heatmap { return nil }
