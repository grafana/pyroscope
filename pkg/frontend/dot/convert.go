package dot

import (
	"bytes"
	"io"

	"github.com/google/pprof/profile"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/v2/pkg/frontend/dot/graph"
	"github.com/grafana/pyroscope/v2/pkg/frontend/dot/report"
)

// FromProfile converts a pprof profile to a DOT graph string.
func FromProfile(p *profilev1.Profile, maxNodes int) (string, error) {
	var buf bytes.Buffer
	if err := WriteFromProfile(&buf, p, maxNodes); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// WriteFromProfile writes a pprof profile as a DOT graph to the given writer.
func WriteFromProfile(w io.Writer, p *profilev1.Profile, maxNodes int) error {
	data, err := p.MarshalVT()
	if err != nil {
		return err
	}
	pr, err := profile.ParseData(data)
	if err != nil {
		return err
	}
	rpt := report.NewDefault(pr, report.Options{NodeCount: maxNodes})
	gr, cfg := report.GetDOT(rpt)
	graph.ComposeDot(w, gr, &graph.DotAttributes{}, cfg)
	return nil
}
