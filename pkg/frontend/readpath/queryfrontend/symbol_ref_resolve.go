package queryfrontend

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grafana/dskit/tracing"
	"golang.org/x/sync/errgroup"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/lidia"
	"github.com/grafana/pyroscope/v2/pkg/model/symbolref"
	validationutil "github.com/grafana/pyroscope/v2/pkg/util/validation"
)

const (
	symbolRefLocationResolved = "resolved"
	symbolRefLocationMiss     = "miss"
	symbolRefLocationTimeout  = "timeout"

	defaultResolveTimeout = 20 * time.Second
	// minResolveConcurrency is a last-resort floor, not a default: the real
	// default lives in pkg/symbolizer.Symbolizer.ResolveConcurrency. This
	// only guards against a Symbolizer implementation that returns a
	// non-positive value, which would otherwise block errgroup.SetLimit
	// forever.
	minResolveConcurrency = 1
)

// resolveSymbolRefs replaces a symbol-ref tree report's tree bytes with a
// plain FunctionName tree, resolving every unresolved {buildID, address}
// reference through the symbolizer. It is a no-op when the report carries no
// symbol-ref table, or the table has nothing unresolved: today's plain tree
// bytes are returned untouched either way.
func (q *QueryFrontend) resolveSymbolRefs(ctx context.Context, tenantIDs []string, report *queryv1.Report, maxNodes int64) error {
	pb := report.GetTree().GetSymbolRefs()
	if pb == nil {
		return nil
	}
	binaries := symbolref.UnresolvedBinaries(pb)
	if len(binaries) == 0 {
		return nil
	}

	span, ctx := tracing.StartSpanFromContext(ctx, "QueryFrontend.resolveSymbolRefs")
	defer span.Finish()
	span.SetTag("binaries", len(binaries))
	span.SetTag("unresolved_references", len(pb.GetUnresolvedAddress()))

	lookup, err := q.resolveBinaries(ctx, tenantIDs, binaries)
	if err != nil {
		return err
	}

	rebuilt, err := symbolref.Rebuild(report.Tree.Tree, pb, lookup.resolve, maxNodes)
	if err != nil {
		return fmt.Errorf("rebuild symbol-ref tree: %w", err)
	}
	report.Tree.Tree = rebuilt
	report.Tree.SymbolRefs = nil
	return nil
}

// symbolRefLookup maps a resolved (buildID, address) location to its frame
// chain; an absent entry means the location has no resolution and Rebuild
// renders the binary!0xaddr fallback for it.
type symbolRefLookup map[symbolRefAddr][]symbolref.Frame

type symbolRefAddr struct {
	buildID string
	addr    uint64
}

func (l symbolRefLookup) resolve(buildID string, addr uint64) []symbolref.Frame {
	return l[symbolRefAddr{buildID, addr}]
}

// binaryResolution is the outcome of resolving one UnresolvedBinary.
type binaryResolution struct {
	binary symbolref.UnresolvedBinary
	// frames is aligned to binary.Addresses; nil when timedOut is true.
	frames [][]lidia.SourceInfoFrame
	// timedOut is true when the binary's own resolve timebox expired while
	// the parent request context was still live: every address is a miss.
	timedOut bool
}

// resolveBinaries resolves every UnresolvedBinary concurrently, bounded by
// the symbolizer's own configured concurrency (the same bound
// SymbolizePprof's debuginfod fetches use) and timeboxed per binary by the
// tenants' resolve-timeout limit.
//
// Resolve errors only when its context is done. A binary whose own timebox
// expires while ctx (the request context) is still live is recorded as a
// miss for all of its addresses rather than failing the request, since the
// timeout only bounds that one binary's resolution cost; if ctx itself is
// done (canceled, or its own deadline reached), the error is propagated and
// the request fails, matching how SymbolizePprof errors are treated at these
// call sites today.
func (q *QueryFrontend) resolveBinaries(ctx context.Context, tenantIDs []string, binaries []symbolref.UnresolvedBinary) (symbolRefLookup, error) {
	timeout := validationutil.SmallestPositiveNonZeroDurationPerTenant(tenantIDs, q.limits.SymbolizerResolveTimeout)
	if timeout <= 0 {
		timeout = defaultResolveTimeout
	}
	concurrency := q.symbolizer.ResolveConcurrency()
	if concurrency < 1 {
		concurrency = minResolveConcurrency
	}

	results := make([]binaryResolution, len(binaries))
	var g errgroup.Group
	g.SetLimit(concurrency)
	for i, binary := range binaries {
		g.Go(func() error {
			binCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			frames, err := q.symbolizer.Resolve(binCtx, binary.BuildID, binary.BinaryName, binary.Addresses)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
					results[i] = binaryResolution{binary: binary, timedOut: true}
					return nil
				}
				return fmt.Errorf("resolve build id %s: %w", binary.BuildID, err)
			}
			results[i] = binaryResolution{binary: binary, frames: frames}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return q.buildLookup(results), nil
}

// buildLookup flattens per-binary resolutions into a single lookup table and
// records the resolved/miss/timeout location counts.
func (q *QueryFrontend) buildLookup(results []binaryResolution) symbolRefLookup {
	lookup := make(symbolRefLookup)
	var resolved, missed, timedOut int
	for _, r := range results {
		for i, addr := range r.binary.Addresses {
			if r.timedOut {
				timedOut++
				continue
			}
			frames := r.frames[i]
			if len(frames) == 0 {
				missed++
				continue
			}
			// lidia returns frames innermost-first (pprof Line order);
			// Rebuild splices chains in root-first order, outermost caller
			// first. Reverse at this boundary.
			out := make([]symbolref.Frame, len(frames))
			for j, f := range frames {
				out[len(frames)-1-j] = symbolref.Frame{Name: f.FunctionName}
			}
			lookup[symbolRefAddr{r.binary.BuildID, addr}] = out
			resolved++
		}
	}
	q.metrics.symbolRefLocationsTotal.WithLabelValues(symbolRefLocationResolved).Add(float64(resolved))
	q.metrics.symbolRefLocationsTotal.WithLabelValues(symbolRefLocationMiss).Add(float64(missed))
	q.metrics.symbolRefLocationsTotal.WithLabelValues(symbolRefLocationTimeout).Add(float64(timedOut))
	return lookup
}
