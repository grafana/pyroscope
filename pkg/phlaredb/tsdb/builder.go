package tsdb

import (
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/prometheus/common/model"
)

type Series struct {
	labels phlaremodel.Labels
	fp     model.Fingerprint
}

type Builder struct {
	series map[string]Series
}

func NewBuilder() *Builder {
	return &Builder{
		series: map[string]Series{},
	}
}
