package symdb

import (
	"context"
	"runtime"
	"sync"

	"github.com/opentracing/opentracing-go"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/sync/errgroup"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/util"
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
	m sync.RWMutex
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
// Only stack traces that belong to the callSite (have the prefix provided)
// will be selected. If empty, the filter is ignored.
// Subtree root location is the last element.
func WithResolverStackTraceSelector(sts *typesv1.StackTraceSelector) ResolverOption {
	return func(r *Resolver) {
		r.sts = sts
	}
}

type lazyPartition struct {
	id uint64

	m       sync.Mutex
	samples *SampleAppender

	fetchOnce sync.Once
	resolver  *Resolver
	reader    PartitionReader
	selection *SelectedStackTraces
	err       error
}

func (p *lazyPartition) fetch(ctx context.Context) error {
	p.fetchOnce.Do(func() {
		p.reader, p.err = p.resolver.s.Partition(ctx, p.id)
		if p.err == nil && p.resolver.sts != nil {
			p.selection = SelectStackTraces(p.reader.Symbols(), p.resolver.sts)
		}
	})
	return p.err
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
	// Wait for all partitions to be fetched / canceled.
	if err := r.g.Wait(); err != nil {
		r.span.SetTag("error", err)
	}
	// Release acquired partition readers.
	var wg sync.WaitGroup
	for _, p := range r.p {
		wg.Add(1)
		p := p
		go func() {
			defer wg.Done()
			if p.reader != nil {
				p.reader.Release()
			}
		}()
	}
	wg.Wait()
	r.span.Finish()
}

// AddSamples adds a collection of stack trace samples to the resolver.
// Samples can be added to partitions concurrently.
func (r *Resolver) AddSamples(partition uint64, s schemav1.Samples) {
	r.withPartitionSamples(partition, func(samples *SampleAppender) {
		samples.AppendMany(s.StacktraceIDs, s.Values)
	})
}

func (r *Resolver) AddSamplesWithSpanSelector(partition uint64, s schemav1.Samples, spanSelector model.SpanSelector) {
	r.withPartitionSamples(partition, func(samples *SampleAppender) {
		for i, sid := range s.StacktraceIDs {
			if _, ok := spanSelector[s.Spans[i]]; ok && sid > 0 {
				samples.Append(sid, s.Values[i])
			}
		}
	})
}

func (r *Resolver) AddSamplesFromParquetRow(partition uint64, stacktraceIDs, values []parquet.Value) {
	r.withPartitionSamples(partition, func(samples *SampleAppender) {
		for i, sid := range stacktraceIDs {
			if s := sid.Uint32(); s > 0 {
				samples.Append(s, values[i].Uint64())
			}
		}
	})
}

func (r *Resolver) AddSamplesWithSpanSelectorFromParquetRow(partition uint64, stacktraces, values, spans []parquet.Value, spanSelector model.SpanSelector) {
	r.withPartitionSamples(partition, func(samples *SampleAppender) {
		for i, sid := range stacktraces {
			spanID := spans[i].Uint64()
			stackID := sid.Uint32()
			if spanID == 0 || stackID == 0 {
				continue
			}
			if _, ok := spanSelector[spanID]; ok {
				samples.Append(stackID, values[i].Uint64())
			}
		}
	})
}

func (r *Resolver) withPartitionSamples(partition uint64, fn func(*SampleAppender)) {
	p := r.partition(partition)
	p.m.Lock()
	defer p.m.Unlock()
	fn(p.samples)
}

func (r *Resolver) CallSiteValues(values *CallSiteValues, partition uint64, samples schemav1.Samples) error {
	p := r.partition(partition)
	if err := p.fetch(r.ctx); err != nil {
		return err
	}
	p.m.Lock()
	defer p.m.Unlock()
	p.selection.CallSiteValues(values, samples)
	return nil
}

func (r *Resolver) CallSiteValuesParquet(values *CallSiteValues, partition uint64, stacktraceID, value []parquet.Value) error {
	p := r.partition(partition)
	if err := p.fetch(r.ctx); err != nil {
		return err
	}
	p.m.Lock()
	defer p.m.Unlock()
	p.selection.CallSiteValuesParquet(values, stacktraceID, value)
	return nil
}

func (r *Resolver) partition(partition uint64) *lazyPartition {
	r.m.RLock()
	p, ok := r.p[partition]
	if ok {
		r.m.RUnlock()
		return p
	}
	r.m.RUnlock()
	r.m.Lock()
	p, ok = r.p[partition]
	if ok {
		r.m.Unlock()
		return p
	}
	p = &lazyPartition{
		id:       partition,
		samples:  NewSampleAppender(),
		resolver: r,
	}
	r.p[partition] = p
	r.m.Unlock()
	// Fetch partition in the background, not blocking the caller.
	// p.reader must be accessed only after p.fetch returns.
	r.g.Go(util.RecoverPanic(func() error {
		return p.fetch(r.ctx)
	}))
	// r.g.Wait() is called at Resolver.Release.
	return p
}

func (r *Resolver) Tree() (*model.Tree, error) {
	span, ctx := opentracing.StartSpanFromContext(r.ctx, "Resolver.Tree")
	defer span.Finish()
	var lock sync.Mutex
	tree := new(model.Tree)
	err := r.withSymbols(ctx, func(symbols *Symbols, appender *SampleAppender) error {
		resolved, err := symbols.Tree(ctx, appender, r.maxNodes, SelectStackTraces(symbols, r.sts))
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
	var p pprof.ProfileMerge
	err := r.withSymbols(ctx, func(symbols *Symbols, appender *SampleAppender) error {
		resolved, err := symbols.Pprof(ctx, appender, r.maxNodes, SelectStackTraces(symbols, r.sts))
		if err != nil {
			return err
		}
		return p.Merge(resolved)
	})
	if err != nil {
		return nil, err
	}
	return p.Profile(), nil
}

func (r *Resolver) withSymbols(ctx context.Context, fn func(*Symbols, *SampleAppender) error) error {
	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(r.c)
	for _, p := range r.p {
		p := p
		g.Go(util.RecoverPanic(func() error {
			if err := p.fetch(ctx); err != nil {
				return err
			}
			return fn(p.reader.Symbols(), p.samples)
		}))
	}
	return g.Wait()
}

func (r *Symbols) Pprof(
	ctx context.Context,
	appender *SampleAppender,
	maxNodes int64,
	selection *SelectedStackTraces,
) (*googlev1.Profile, error) {
	return buildPprof(ctx, r, appender.Samples(), maxNodes, selection)
}

func (r *Symbols) Tree(
	ctx context.Context,
	appender *SampleAppender,
	maxNodes int64,
	selection *SelectedStackTraces,
) (*model.Tree, error) {
	return buildTree(ctx, r, appender, maxNodes, selection)
}
