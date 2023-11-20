package symdb

import (
	"context"
	"runtime"
	"sync"

	"github.com/google/pprof/profile"
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

func (r *Resolver) Profile() (*profile.Profile, error) {
	span, ctx := opentracing.StartSpanFromContext(r.ctx, "Resolver.Profile")
	defer span.Finish()
	var lock sync.Mutex
	profiles := make([]*profile.Profile, 0, len(r.p))
	err := r.withSymbols(ctx, func(symbols *Symbols, samples schemav1.Samples) error {
		resolved, err := symbols.Profile(ctx, samples)
		if err != nil {
			return err
		}
		lock.Lock()
		profiles = append(profiles, resolved)
		lock.Unlock()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return profile.Merge(profiles)
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

func (r *Symbols) Pprof(ctx context.Context, samples schemav1.Samples, maxNodes int64) (*googlev1.Profile, error) {
	t := pprofProtoSymbols{maxNodes: maxNodes}
	t.init(r, samples)
	if err := r.Stacktraces.ResolveStacktraceLocations(ctx, &t, samples.StacktraceIDs); err != nil {
		return nil, err
	}
	return t.buildPprof(), nil
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

type treeSymbols struct {
	symbols *Symbols
	samples *schemav1.Samples
	tree    *model.Tree
	lines   []string
	cur     int
}

var treeSymbolsPool = sync.Pool{
	New: func() any { return new(treeSymbols) },
}

func treeSymbolsFromPool() *treeSymbols {
	return treeSymbolsPool.Get().(*treeSymbols)
}

func (r *treeSymbols) reset() {
	r.symbols = nil
	r.samples = nil
	r.tree = nil
	r.lines = r.lines[:0]
	r.cur = 0
	treeSymbolsPool.Put(r)
}

func (r *treeSymbols) init(symbols *Symbols, samples schemav1.Samples) {
	r.symbols = symbols
	r.samples = &samples
	r.tree = new(model.Tree)
}

func (r *treeSymbols) InsertStacktrace(_ uint32, locations []int32) {
	r.lines = r.lines[:0]
	for i := len(locations) - 1; i >= 0; i-- {
		lines := r.symbols.Locations[locations[i]].Line
		for j := len(lines) - 1; j >= 0; j-- {
			f := r.symbols.Functions[lines[j].FunctionId]
			r.lines = append(r.lines, r.symbols.Strings[f.Name])
		}
	}
	r.tree.InsertStack(int64(r.samples.Values[r.cur]), r.lines...)
	r.cur++
}

func (r *Symbols) Profile(ctx context.Context, samples schemav1.Samples) (*profile.Profile, error) {
	t := pprofResolveFromPool()
	defer t.reset()
	t.init(r, samples)
	if err := r.Stacktraces.ResolveStacktraceLocations(ctx, t, samples.StacktraceIDs); err != nil {
		return nil, err
	}
	t.incrementIDs()
	return t.profile, nil
}

type pprofSymbols struct {
	profile *profile.Profile
	symbols *Symbols
	samples *schemav1.Samples
	cur     int

	locations []*profile.Location
	mappings  []*profile.Mapping
	functions []*profile.Function
}

var pprofSymbolsPool = sync.Pool{
	New: func() any { return new(pprofSymbols) },
}

func pprofResolveFromPool() *pprofSymbols {
	return pprofSymbolsPool.Get().(*pprofSymbols)
}

func (r *pprofSymbols) reset() {
	r.profile = nil
	r.symbols = nil
	r.samples = nil
	r.cur = 0
	clear(r.locations)
	clear(r.mappings)
	clear(r.functions)
	pprofSymbolsPool.Put(r)
}

func (r *pprofSymbols) init(symbols *Symbols, samples schemav1.Samples) {
	r.symbols = symbols
	r.samples = &samples
	r.profile = &profile.Profile{
		Sample:     make([]*profile.Sample, len(samples.StacktraceIDs)),
		PeriodType: new(profile.ValueType),
	}
	r.locations = grow(r.locations, len(r.symbols.Locations))
	r.mappings = grow(r.mappings, len(r.symbols.Mappings))
	r.functions = grow(r.functions, len(r.symbols.Functions))
}

func grow[T any](s []T, n int) []T {
	if cap(s) < n {
		s = make([]T, n)
	}
	s = s[:n]
	return s
}

func clear[T any](s []T) {
	var zero T
	for i := range s {
		s[i] = zero
	}
}

func (r *pprofSymbols) InsertStacktrace(_ uint32, locations []int32) {
	sample := &profile.Sample{
		Location: make([]*profile.Location, len(locations)),
		Value:    []int64{int64(r.samples.Values[r.cur])},
	}
	for j, loc := range locations {
		sample.Location[j] = r.location(loc)
	}
	r.profile.Sample[r.cur] = sample
	r.cur++
}

func (r *pprofSymbols) location(i int32) *profile.Location {
	if x := r.locations[i]; x != nil {
		return x
	}
	loc := r.inMemoryLocationToPprof(r.symbols.Locations[i])
	r.profile.Location = append(r.profile.Location, loc)
	r.locations[i] = loc
	return loc
}

func (r *pprofSymbols) mapping(i uint32) *profile.Mapping {
	if x := r.mappings[i]; x != nil {
		return x
	}
	m := r.inMemoryMappingToPprof(r.symbols.Mappings[i])
	r.profile.Mapping = append(r.profile.Mapping, m)
	r.mappings[i] = m
	return m
}

func (r *pprofSymbols) function(i uint32) *profile.Function {
	if x := r.functions[i]; x != nil {
		return x
	}
	f := r.inMemoryFunctionToPprof(r.symbols.Functions[i])
	r.profile.Function = append(r.profile.Function, f)
	r.functions[i] = f
	return f
}

func (r *pprofSymbols) inMemoryLocationToPprof(m *schemav1.InMemoryLocation) *profile.Location {
	x := &profile.Location{
		ID:       m.Id,
		Mapping:  r.mapping(m.MappingId),
		Address:  m.Address,
		IsFolded: m.IsFolded,
	}
	x.Line = make([]profile.Line, len(m.Line))
	for i, line := range m.Line {
		x.Line[i] = profile.Line{
			Function: r.function(line.FunctionId),
			Line:     int64(line.Line),
		}
	}
	return x
}

func (r *pprofSymbols) inMemoryMappingToPprof(m *schemav1.InMemoryMapping) *profile.Mapping {
	return &profile.Mapping{
		ID:              m.Id,
		Start:           m.MemoryStart,
		Limit:           m.MemoryLimit,
		Offset:          m.FileOffset,
		File:            r.symbols.Strings[m.Filename],
		BuildID:         r.symbols.Strings[m.BuildId],
		HasFunctions:    m.HasFunctions,
		HasFilenames:    m.HasFilenames,
		HasLineNumbers:  m.HasLineNumbers,
		HasInlineFrames: m.HasInlineFrames,
	}
}

func (r *pprofSymbols) inMemoryFunctionToPprof(m *schemav1.InMemoryFunction) *profile.Function {
	return &profile.Function{
		ID:         m.Id,
		Name:       r.symbols.Strings[m.Name],
		SystemName: r.symbols.Strings[m.SystemName],
		Filename:   r.symbols.Strings[m.Filename],
		StartLine:  int64(m.StartLine),
	}
}

func (r *pprofSymbols) incrementIDs() {
	for i, l := range r.profile.Location {
		l.ID = uint64(i) + 1
	}
	for i, f := range r.profile.Function {
		f.ID = uint64(i) + 1
	}
	for i, m := range r.profile.Mapping {
		m.ID = uint64(i) + 1
	}
}
