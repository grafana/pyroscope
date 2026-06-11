package querybackend

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
)

// Symbolizer symbolizes unsymbolized profiles. The context must carry the
// owning tenant of the profile being symbolized.
type Symbolizer interface {
	SymbolizePprof(ctx context.Context, profile *googlev1.Profile) error
}

// Limits gates symbolization per tenant; size limits are enforced by the
// symbolizer itself.
type Limits interface {
	SymbolizerEnabled(tenantID string) bool
}

// shouldSymbolizeDataset reports whether the dataset must be symbolized
// before its report is built; symbolized datasets keep the native path.
func (q *queryContext) shouldSymbolizeDataset() bool {
	return q.req.src.GetOptions().GetSymbolize() &&
		q.symbolizer != nil &&
		q.limits != nil &&
		q.limits.SymbolizerEnabled(q.ds.TenantID()) &&
		datasetUnsymbolized(q.obj.Metadata(), q.ds.Metadata())
}

// datasetUnsymbolized reports whether any of the dataset's label sets carries
// __unsymbolized__="true": compacted datasets accumulate the label sets of
// all their sources.
func datasetUnsymbolized(md *metastorev1.BlockMeta, ds *metastorev1.Dataset) bool {
	pairs := metadata.LabelPairs(ds.Labels)
	for pairs.Next() {
		p := pairs.At()
		for k := 0; k+1 < len(p); k += 2 {
			n, v := p[k], p[k+1]
			if n < 0 || int(n) >= len(md.StringTable) || v < 0 || int(v) >= len(md.StringTable) {
				continue
			}
			if md.StringTable[n] == metadata.LabelNameUnsymbolized && md.StringTable[v] == "true" {
				return true
			}
		}
	}
	return false
}

// symbolizePprof symbolizes on behalf of the dataset's owning tenant:
// the symbolizer cache and limits are tenant-scoped, and in multi-tenant
// blocks the request tenant is not necessarily the dataset tenant.
func (q *queryContext) symbolizePprof(profile *googlev1.Profile) error {
	trace.SpanFromContext(q.ctx).SetAttributes(attribute.Bool("symbolize_dataset", true))
	err := q.symbolizer.SymbolizePprof(tenant.InjectTenantID(q.ctx, q.ds.TenantID()), profile)
	status := "success"
	if err != nil {
		status = "error"
	}
	q.metrics.symbolizedDatasets.WithLabelValues(status).Inc()
	return err
}
