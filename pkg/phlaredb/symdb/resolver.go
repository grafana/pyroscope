package symdb

import (
	"context"
	"runtime"
	"sync"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/sync/errgroup"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
)

// Resolver converts stack trace samples to one of the profile
// formats, such as tree or pprof.
//
// Resolver asynchronously loads symbols for each partition as
// they are added with AddSamples or Partition calls.
//
// A new Resolver must be created for each profile.
type Resolver struct {
	ctx    context.Context
	cancel context.CancelFunc
	span   opentracing.Span

	s SymbolsReader
	g *errgroup.Group
	c int
	m sync.Mutex
	p map[uint64]*lazyPartition

	maxNodes int64
	sts      *typesv1.StackTraceSelector
}

type ResolverOption func(*Resolver)

// WithResolverMaxConcurrent specifies how many partitions
// can be resolved concurrently.
func WithResolverMaxConcurrent(n int) ResolverOption {
	return func(r *Resolver) {
		r.c = n
	}
}

// WithResolverMaxNodes specifies the desired maximum number
// of nodes the resulting profile should include.
func WithResolverMaxNodes(n int64) ResolverOption {
	return func(r *Resolver) {
		r.maxNodes = n
	}
}

// WithResolverStackTraceSelector specifies the stack trace selector.
// Only stack traces that belong to the subtree (have the prefix provided)
// will be selected. If empty, the filter is ignored.
// Subtree root location is the last element.
func WithResolverStackTraceSelector(sts *typesv1.StackTraceSelector) ResolverOption {
	return func(r *Resolver) {
		r.sts = sts
	}
}

type lazyPartition struct {
	m       sync.Mutex
	samples map[uint32]int64

	id     uint64
	reader chan PartitionReader
	err    chan error
	done   chan struct{}
}

func NewResolver(ctx context.Context, s SymbolsReader, opts ...ResolverOption) *Resolver {
	r := Resolver{
		s: s,
		c: runtime.GOMAXPROCS(-1),
		p: make(map[uint64]*lazyPartition),
	}
	for _, opt := range opts {
		opt(&r)
	}
	r.span, r.ctx = opentracing.StartSpanFromContext(ctx, "NewResolver")
	r.ctx, r.cancel = context.WithCancel(r.ctx)
	r.g, r.ctx = errgroup.WithContext(r.ctx)
	return &r
}

func (r *Resolver) Release() {
	r.cancel()
	// The error is already sent to the caller.
	_ = r.g.Wait()
	r.span.Finish()
}

// AddSamples adds a collection of stack trace samples to the resolver.
// Samples can be added to partitions concurrently.
func (r *Resolver) AddSamples(partition uint64, s schemav1.Samples) {
	r.WithPartitionSamples(partition, func(samples map[uint32]int64) {
		for i, sid := range s.StacktraceIDs {
			if sid > 0 {
				samples[sid] += int64(s.Values[i])
			}
		}
	})
}

func (r *Resolver) AddSamplesFromParquetRow(partition uint64, stacktraceIDs, values []parquet.Value) {
	r.WithPartitionSamples(partition, func(samples map[uint32]int64) {
		for i, sid := range stacktraceIDs {
			samples[sid.Uint32()] += values[i].Int64()
		}
	})
}

func (r *Resolver) AddSamplesWithSpanSelector(partition uint64, s schemav1.Samples, spanSelector model.SpanSelector) {
	r.WithPartitionSamples(partition, func(samples map[uint32]int64) {
		for i, sid := range s.StacktraceIDs {
			if _, ok := spanSelector[s.Spans[i]]; ok {
				samples[sid] += int64(s.Values[i])
			}
		}
	})
}

func (r *Resolver) WithPartitionSamples(partition uint64, fn func(map[uint32]int64)) {
	p := r.partition(partition)
	p.m.Lock()
	defer p.m.Unlock()
	fn(p.samples)
}

func (r *Resolver) partition(partition uint64) *lazyPartition {
	r.m.Lock()
	p, ok := r.p[partition]
	if ok {
		r.m.Unlock()
		return p
	}
	p = &lazyPartition{
		id:      partition,
		samples: make(map[uint32]int64),
		err:     make(chan error),
		done:    make(chan struct{}),
		reader:  make(chan PartitionReader, 1),
	}
	r.p[partition] = p
	r.m.Unlock()
	r.g.Go(func() error {
		return r.acquirePartition(p)
	})
	// r.g.Wait() is only called at Resolver.Release.
	return p
}

func (r *Resolver) acquirePartition(p *lazyPartition) error {
	pr, err := r.s.Partition(r.ctx, p.id)
	if err != nil {
		r.span.LogFields(log.String("err", err.Error()))
		select {
		case <-r.ctx.Done():
			return r.ctx.Err()
		case p.err <- err:
			// Signal the partition receiver
			// about the failure, so it won't
			// block and return early.
			return err
		}
	}
	// We've acquired the partition and must release it
	// once resolution finished or canceled.
	select {
	case p.reader <- pr:
		// We transferred ownership to the recipient,
		// which is now responsible for releasing the
		// partition.
		<-p.done
	case <-r.ctx.Done():
		// We still own the partition and must release
		// it on our own. It's guaranteed that p.c receiver
		// has no access to the partition.
		pr.Release()
		return r.ctx.Err()
	}
	return nil
}

func (r *Resolver) Tree() (*model.Tree, error) {
	span, ctx := opentracing.StartSpanFromContext(r.ctx, "Resolver.Tree")
	defer span.Finish()
	var lock sync.Mutex
	tree := new(model.Tree)
	err := r.withSymbols(ctx, func(symbols *Symbols, samples schemav1.Samples) error {
		resolved, err := symbols.Tree(ctx, samples)
		if err != nil {
			return err
		}
		lock.Lock()
		tree.Merge(resolved)
		lock.Unlock()
		return nil
	})
	return tree, err
}

func (r *Resolver) Pprof() (*googlev1.Profile, error) {
	span, ctx := opentracing.StartSpanFromContext(r.ctx, "Resolver.Pprof")
	defer span.Finish()
	var lock sync.Mutex
	var p pprof.ProfileMerge
	err := r.withSymbols(ctx, func(symbols *Symbols, samples schemav1.Samples) error {
		resolved, err := symbols.Pprof(ctx, samples, r.maxNodes, r.sts)
		if err != nil {
			return err
		}
		lock.Lock()
		defer lock.Unlock()
		return p.Merge(resolved)
	})
	return p.Profile(), err
}

func (r *Resolver) withSymbols(ctx context.Context, fn func(*Symbols, schemav1.Samples) error) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(r.c)
	for _, p := range r.p {
		p := p
		g.Go(func() error {
			defer close(p.done)
			select {
			case err := <-p.err:
				return err
			case <-ctx.Done():
				return ctx.Err()
			case pr := <-p.reader:
				defer pr.Release()
				return fn(pr.Symbols(), schemav1.NewSamplesFromMap(p.samples))
			}
		})
	}
	return g.Wait()
}

type pprofBuilder interface {
	StacktraceInserter
	init(*Symbols, schemav1.Samples)
	buildPprof() *googlev1.Profile
}

func (r *Symbols) Pprof(
	ctx context.Context,
	samples schemav1.Samples,
	maxNodes int64,
	sts *typesv1.StackTraceSelector,
) (*googlev1.Profile, error) {
	// By default, we use a builder that's optimized
	// for the simplest case: we take all the source
	// stack traces unchanged.
	var b pprofBuilder = new(pprofProtoSymbols)
	// If a stack trace selector is specified,
	// check if such a profile can exist at all.
	var subtree []uint32
	if locs := sts.GetCallSite(); len(locs) > 0 {
		if subtree = findCallSite(r, locs); len(subtree) == 0 {
			return b.buildPprof(), nil
		}
	}
	// Truncation is applicable when there is an explicit
	// limit on the number of the nodes in the profile, or
	// if stack traces should be filtered.
	if maxNodes > 0 || len(subtree) > 0 {
		b = &pprofProtoTruncatedSymbols{
			maxNodes: maxNodes,
			subtree:  subtree,
		}
	}
	b.init(r, samples)
	if err := r.Stacktraces.ResolveStacktraceLocations(ctx, b, samples.StacktraceIDs); err != nil {
		return nil, err
	}
	return b.buildPprof(), nil
}

func (r *Symbols) Tree(ctx context.Context, samples schemav1.Samples) (*model.Tree, error) {
	t := treeSymbolsFromPool()
	defer t.reset()
	t.init(r, samples)
	if err := r.Stacktraces.ResolveStacktraceLocations(ctx, t, samples.StacktraceIDs); err != nil {
		return nil, err
	}
	return t.tree, nil
}
