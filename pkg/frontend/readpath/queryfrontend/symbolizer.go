package queryfrontend

import (
	"context"
	"fmt"
	"slices"

	"github.com/grafana/dskit/tracing"
	"github.com/prometheus/prometheus/model/labels"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
)

// TODO: Symbolization currently happens in the query frontend as a post-processing step.
// Eventually it should move into the query backends and become part of the query plan,
// so that symbolization can be distributed and executed closer to the data.

// backendTreeSymbolizer allows to symbolize FunctionName Tree queries, by converting them into pprof queries, symbolize them and converting them back.
type backendTreeSymbolizer struct {
	upstream   QueryBackend
	symbolizer Symbolizer
}

func (b *backendTreeSymbolizer) Invoke(ctx context.Context, req *queryv1.InvokeRequest) (resp *queryv1.InvokeResponse, err error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "backendTreeSymbolizer.Invoke")
	defer func() {
		if err != nil {
			span.LogError(err)
			span.SetError()
		}
		span.Finish()
	}()
	span.SetTag("query_count", len(req.Query))

	modifiedReq := req.CloneVT()

	// check all queries for the ones using tree
	for _, q := range modifiedReq.Query {
		// If this is a TREE query, convert it to PPROF
		if q.QueryType == queryv1.QueryType_QUERY_TREE {
			q.QueryType = queryv1.QueryType_QUERY_PPROF
			q.Pprof = &queryv1.PprofQuery{
				MaxNodes:           q.Tree.GetMaxNodes(),
				ProfileIdSelector:  q.Tree.GetProfileIdSelector(),
				StackTraceSelector: q.Tree.GetStackTraceSelector(),
				// SpanSelector is not forwarded: PprofQuery has no span_selector field.
				// To support span-filtered symbolization, span_selector must be added to
				// the PprofQuery proto and the backend must handle it.
			}
			q.Tree = nil
		}
	}

	// invoke modified request
	resp, err = b.upstream.Invoke(ctx, modifiedReq)
	if err != nil {
		return nil, err
	}
	span.SetTag("report_count", len(resp.Reports))

	if len(req.Query) != len(resp.Reports) {
		return nil, fmt.Errorf("query/report count mismatch: %d queries but %d reports",
			len(req.Query), len(resp.Reports))
	}

	for i, r := range resp.Reports {
		if r.Pprof == nil || r.Pprof.Pprof == nil {
			continue
		}

		var prof googlev1.Profile
		if err := pprof.Unmarshal(r.Pprof.Pprof, &prof); err != nil {
			return nil, fmt.Errorf("failed to unmarshal profile: %w", err)
		}

		if err := b.symbolizer.SymbolizePprof(ctx, &prof); err != nil {
			return nil, fmt.Errorf("failed to symbolize profile: %w", err)
		}

		// Convert back to tree if originally a tree
		if i < len(req.Query) && req.Query[i].QueryType == queryv1.QueryType_QUERY_TREE {
			treeBytes, err := model.TreeFromBackendProfile(&prof, req.Query[i].Tree.GetMaxNodes())
			if err != nil {
				return nil, fmt.Errorf("failed to build tree: %w", err)
			}
			r.Tree = &queryv1.TreeReport{Tree: treeBytes}
			r.ReportType = queryv1.ReportType_REPORT_TREE
			r.Pprof = nil
		}
	}

	return resp, nil
}

// hasUnsymbolizedProfiles checks if a block has unsymbolized profiles
func (q *QueryFrontend) hasUnsymbolizedProfiles(block *metastorev1.BlockMeta) bool {
	matcher, err := labels.NewMatcher(labels.MatchEqual, metadata.LabelNameUnsymbolized, "true")
	if err != nil {
		return false
	}

	return len(slices.Collect(metadata.FindDatasets(block, matcher))) > 0
}

// shouldSymbolize determines if we should symbolize profiles based on tenant settings
// and the unsymbolized label on the returned blocks.
//
// Limitation: queries without a strict service_name label selector fall back to the
// tenant-wide TSDB index blocks (see QueryMetadata). Those blocks do not carry
// per-dataset unsymbolized=true labels, so this function will return false and
// symbolization will be silently skipped for such queries even when unsymbolized
// profiles exist.
func (q *QueryFrontend) shouldSymbolize(ctx context.Context, tenants []string, blocks []*metastorev1.BlockMeta) bool {
	otelSpan := oteltrace.SpanFromContext(ctx)

	if q.symbolizer == nil {
		return false
	}

	for _, t := range tenants {
		if !q.limits.SymbolizerEnabled(t) {
			return false
		}
	}

	blocksWithUnsymbolized := 0
	for _, block := range blocks {
		if q.hasUnsymbolizedProfiles(block) {
			blocksWithUnsymbolized++
		}
	}

	otelSpan.SetAttributes(
		attribute.Int("blocks_with_unsymbolized", blocksWithUnsymbolized),
		attribute.Int("total_blocks", len(blocks)),
	)

	return blocksWithUnsymbolized > 0
}
