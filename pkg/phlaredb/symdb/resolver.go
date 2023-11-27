package symdb

import (
	"context"
	"runtime"
	"sync"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"golang.org/x/sync/errgroup"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
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
}

type ResolverOption func(*Resolver)

// WithMaxConcurrent specifies how many partitions
// can be resolved concurrently.
func WithMaxConcurrent(n int) ResolverOption {
	return func(r *Resolver) {
		r.c = n
	}
}

type lazyPartition struct {
	id      uint64
	reader  chan PartitionReader
	samples map[uint32]int64
	err     chan error
	done    chan struct{}
}

func NewResolver(ctx context.Context, s SymbolsReader) *Resolver {
	r := Resolver{
		s: s,
		c: runtime.GOMAXPROCS(-1),
		p: make(map[uint64]*lazyPartition),
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
// Samples can be added to different partitions concurrently, but modification
// of the same partition is not thread-safe.
func (r *Resolver) AddSamples(partition uint64, s schemav1.Samples) {
	p := r.Partition(partition)
	for i, sid := range s.StacktraceIDs {
		if sid > 0 {
			p[sid] += int64(s.Values[i])
		}
	}
}

func (r *Resolver) AddSamplesWithSpanSelector(partition uint64, s schemav1.Samples, spanSelector model.SpanSelector) {
	p := r.Partition(partition)
	for i, sid := range s.StacktraceIDs {
		if _, ok := spanSelector[s.Spans[i]]; ok {
			p[sid] += int64(s.Values[i])
		}
	}
}

// Partition returns map of samples corresponding to the partition.
// The function initializes symbols of the partition on the first occurrence.
// The call is thread-safe, but access to the returned map is not.
func (r *Resolver) Partition(partition uint64) map[uint32]int64 {
	r.m.Lock()
	p, ok := r.p[partition]
	if ok {
		r.m.Unlock()
		return p.samples
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
	return p.samples
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

func (r *Resolver) Pprof(maxNodes int64) (*googlev1.Profile, error) {
	span, ctx := opentracing.StartSpanFromContext(r.ctx, "Resolver.Pprof")
	defer span.Finish()
	var lock sync.Mutex
	var p pprof.ProfileMerge
	err := r.withSymbols(ctx, func(symbols *Symbols, samples schemav1.Samples) error {
		resolved, err := symbols.Pprof(ctx, samples, maxNodes)
		if err != nil {
			return err
		}
		lock.Lock()
		err = p.Merge(resolved)
		lock.Unlock()
		return err
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

func (r *Symbols) Pprof(ctx context.Context, samples schemav1.Samples, maxNodes int64) (*googlev1.Profile, error) {
	var b pprofBuilder = new(pprofProtoSymbols)
	if maxNodes > 0 {
		b = &pprofProtoTruncatedSymbols{maxNodes: maxNodes}
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
