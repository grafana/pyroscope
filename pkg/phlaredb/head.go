package phlaredb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/status"
	"github.com/google/pprof/profile"
	"github.com/google/uuid"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/samber/lo"
	"go.uber.org/atomic"
	"google.golang.org/grpc/codes"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/iter"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	"github.com/grafana/phlare/pkg/phlaredb/block"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
	"github.com/grafana/phlare/pkg/phlaredb/symdb"
)

func copySlice[T any](in []T) []T {
	out := make([]T, len(in))
	copy(out, in)
	return out
}

type idConversionTable map[int64]int64

// nolint unused
func (t idConversionTable) rewrite(idx *int64) {
	pos := *idx
	var ok bool
	*idx, ok = t[pos]
	if !ok {
		panic(fmt.Sprintf("unable to rewrite index %d", pos))
	}
}

// nolint unused
func (t idConversionTable) rewriteUint64(idx *uint64) {
	pos := *idx
	v, ok := t[int64(pos)]
	if !ok {
		panic(fmt.Sprintf("unable to rewrite index %d", pos))
	}
	*idx = uint64(v)
}

// nolint unused
func (t idConversionTable) rewriteUint32(idx *uint32) {
	pos := *idx
	v, ok := t[int64(pos)]
	if !ok {
		panic(fmt.Sprintf("unable to rewrite index %d", pos))
	}
	*idx = uint32(v)
}

type Models interface {
	*schemav1.Profile | *schemav1.InMemoryProfile |
		*profilev1.Location | *schemav1.InMemoryLocation |
		*profilev1.Function | *schemav1.InMemoryFunction |
		*profilev1.Mapping | *schemav1.InMemoryMapping |
		*schemav1.StoredString | string |
		*schemav1.Stacktrace
}

func emptyRewriter() *rewriter {
	return &rewriter{
		strings: []int64{0},
	}
}

// rewriter contains slices to rewrite the per profile reference into per head references.
type rewriter struct {
	strings stringConversionTable
	// nolint unused
	functions idConversionTable
	// nolint unused
	mappings idConversionTable
	// nolint unused
	locations idConversionTable
}

type storeHelper[M Models] interface {
	// some Models contain their own IDs within the struct, this allows to set them and keep track of the preexisting ID. It should return the oldID that is supposed to be rewritten.
	setID(existingSliceID uint64, newID uint64, element M) uint64

	// size returns a (rough estimation) of the size of a single element M
	size(M) uint64

	// clone copies parts that are not optimally sized from protobuf parsing
	clone(M) M

	rewrite(*rewriter, M) error
}

type Helper[M Models, K comparable] interface {
	storeHelper[M]
	key(M) K
	addToRewriter(*rewriter, idConversionTable)
}

type Table interface {
	Name() string
	Size() uint64       // Size estimates the uncompressed byte size of the table in memory and on disk.
	MemorySize() uint64 // MemorySize estimates the uncompressed byte size of the table in memory.
	Init(path string, cfg *ParquetConfig, metrics *headMetrics) error
	Flush(context.Context) (numRows uint64, numRowGroups uint64, err error)
	Close() error
}

type Head struct {
	logger  log.Logger
	metrics *headMetrics
	stopCh  chan struct{}
	wg      sync.WaitGroup

	headPath  string // path while block is actively appended to
	localPath string // path once block has been cut

	inFlightProfiles sync.WaitGroup // ongoing ingestion requests.
	flushCh          chan struct{}  // this channel is closed once the Head should be flushed, should be used externally
	flushForcedTimer *time.Timer    // this timer will phlare after the maximum

	metaLock sync.RWMutex
	meta     *block.Meta

	parquetConfig *ParquetConfig
	strings       deduplicatingSlice[string, string, *stringsHelper, *schemav1.StringPersister]
	mappings      deduplicatingSlice[*schemav1.InMemoryMapping, mappingsKey, *mappingsHelper, *schemav1.MappingPersister]
	functions     deduplicatingSlice[*schemav1.InMemoryFunction, functionsKey, *functionsHelper, *schemav1.FunctionPersister]
	locations     deduplicatingSlice[*schemav1.InMemoryLocation, locationsKey, *locationsHelper, *schemav1.LocationPersister]
	symbolDB      *symdb.SymDB

	profiles     *profileStore
	totalSamples *atomic.Uint64
	tables       []Table
	delta        *deltaProfiles

	limiter TenantLimiter
}

const (
	pathHead          = "head"
	pathLocal         = "local"
	defaultFolderMode = 0o755
)

func NewHead(phlarectx context.Context, cfg Config, limiter TenantLimiter) (*Head, error) {
	// todo if tenantLimiter is nil ....
	parquetConfig := *defaultParquetConfig
	h := &Head{
		logger:  phlarecontext.Logger(phlarectx),
		metrics: contextHeadMetrics(phlarectx),

		stopCh: make(chan struct{}),

		meta:         block.NewMeta(),
		totalSamples: atomic.NewUint64(0),

		flushCh:          make(chan struct{}),
		flushForcedTimer: time.NewTimer(cfg.MaxBlockDuration),

		parquetConfig: &parquetConfig,
		limiter:       limiter,
	}
	h.headPath = filepath.Join(cfg.DataPath, pathHead, h.meta.ULID.String())
	h.localPath = filepath.Join(cfg.DataPath, pathLocal, h.meta.ULID.String())

	if cfg.Parquet != nil {
		h.parquetConfig = cfg.Parquet
	}

	h.parquetConfig.MaxRowGroupBytes = cfg.RowGroupTargetSize

	// ensure folder is writable
	err := os.MkdirAll(h.headPath, defaultFolderMode)
	if err != nil {
		return nil, err
	}

	// create profile store
	h.profiles = newProfileStore(phlarectx)

	h.tables = []Table{
		&h.strings,
		&h.mappings,
		&h.functions,
		&h.locations,
		h.profiles,
	}
	for _, t := range h.tables {
		if err := t.Init(h.headPath, h.parquetConfig, h.metrics); err != nil {
			return nil, err
		}
	}
	h.symbolDB = symdb.NewSymDB(
		symdb.DefaultConfig().
			WithDirectory(filepath.Join(h.headPath, symdb.DefaultDirName)),
	)

	h.delta = newDeltaProfiles()

	h.wg.Add(1)
	go h.loop()

	return h, nil
}

func (h *Head) MemorySize() uint64 {
	var size uint64
	// TODO: Estimate size of TSDB index
	for _, t := range h.tables {
		size += t.MemorySize()
	}
	size += h.symbolDB.MemorySize()
	return size
}

func (h *Head) Size() uint64 {
	var size uint64
	// TODO: Estimate size of TSDB index
	for _, t := range h.tables {
		size += t.Size()
	}
	size += h.symbolDB.Size()

	return size
}

func (h *Head) loop() {
	defer h.wg.Done()

	tick := time.NewTicker(5 * time.Second)
	defer func() {
		tick.Stop()
		h.flushForcedTimer.Stop()
	}()

	for {
		select {
		case <-h.flushForcedTimer.C:
			h.metrics.flushedBlocksReasons.WithLabelValues("max-duration").Inc()
			level.Debug(h.logger).Log("msg", "max block duration reached, flush to disk")
			close(h.flushCh)
			return
		case <-tick.C:
			if currentSize := h.Size(); currentSize > h.parquetConfig.MaxBlockBytes {
				h.metrics.flushedBlocksReasons.WithLabelValues("max-block-bytes").Inc()
				level.Debug(h.logger).Log(
					"msg", "max block bytes reached, flush to disk",
					"max_size", humanize.Bytes(h.parquetConfig.MaxBlockBytes),
					"current_head_size", humanize.Bytes(currentSize),
				)
				close(h.flushCh)
				return
			}
		case <-h.stopCh:
			return
		}
	}
}

func (h *Head) convertSamples(_ context.Context, r *rewriter, stacktracePartition uint64, in []*profilev1.Sample) []schemav1.Samples {
	if len(in) == 0 {
		return nil
	}

	// populate output
	var (
		out            = make([]schemav1.Samples, len(in[0].Value))
		stacktraces    = make([]*schemav1.Stacktrace, len(in))
		stacktracesIds = uint32SlicePool.Get()
	)

	for idxType := range out {
		out[idxType] = schemav1.Samples{
			Values:        make([]uint64, len(in)),
			StacktraceIDs: make([]uint32, len(in)),
		}
	}

	for idxSample := range in {
		// populate samples
		for idxType := range out {
			out[idxType].Values[idxSample] = uint64(in[idxSample].Value[idxType])
		}

		// build full stack traces
		stacktraces[idxSample] = &schemav1.Stacktrace{
			// no copySlice necessary at this point,stacktracesHelper.clone
			// will copy it, if it is required to be retained.
			LocationIDs: in[idxSample].LocationId,
		}
		for i := range stacktraces[idxSample].LocationIDs {
			r.locations.rewriteUint64(&stacktraces[idxSample].LocationIDs[i])
		}
	}
	appender := h.symbolDB.MappingWriter(stacktracePartition).StacktraceAppender()
	defer appender.Release()

	if cap(stacktracesIds) < len(stacktraces) {
		stacktracesIds = make([]uint32, len(stacktraces))
	}
	stacktracesIds = stacktracesIds[:len(stacktraces)]
	defer uint32SlicePool.Put(stacktracesIds)

	appender.AppendStacktrace(stacktracesIds, stacktraces)

	h.metrics.sizeBytes.WithLabelValues("stacktraces").Set(float64(h.symbolDB.MemorySize()))

	// reference stacktraces
	for idxType := range out {
		for idxSample := range out[idxType].StacktraceIDs {
			out[idxType].StacktraceIDs[idxSample] = stacktracesIds[int64(idxSample)]
		}
		compacted := out[idxType].Compact(true)
		if compacted.Len() != out[idxType].Len() {
			compacted = compacted.Clone()
		}
		out[idxType] = compacted
	}

	return out
}

func (h *Head) Ingest(ctx context.Context, p *profilev1.Profile, id uuid.UUID, externalLabels ...*typesv1.LabelPair) error {
	labels, seriesFingerprints := labelsForProfile(p, externalLabels...)

	for i, fp := range seriesFingerprints {
		if err := h.limiter.AllowProfile(fp, labels[i], p.TimeNanos); err != nil {
			return err
		}
	}

	// determine the stacktraces partition ID
	stacktracePartition := phlaremodel.StacktracePartitionFromProfile(labels, p)

	metricName := phlaremodel.Labels(externalLabels).Get(model.MetricNameLabel)

	// create a rewriter state
	rewrites := &rewriter{}

	if err := h.strings.ingest(ctx, p.StringTable, rewrites); err != nil {
		return err
	}

	mappings := make([]*schemav1.InMemoryMapping, len(p.Mapping))
	for i, v := range p.Mapping {
		mappings[i] = &schemav1.InMemoryMapping{
			Id:              v.Id,
			MemoryStart:     v.MemoryStart,
			MemoryLimit:     v.MemoryLimit,
			FileOffset:      v.FileOffset,
			Filename:        uint32(v.Filename),
			BuildId:         uint32(v.BuildId),
			HasFunctions:    v.HasFunctions,
			HasFilenames:    v.HasFilenames,
			HasLineNumbers:  v.HasLineNumbers,
			HasInlineFrames: v.HasInlineFrames,
		}
	}
	if err := h.mappings.ingest(ctx, mappings, rewrites); err != nil {
		return err
	}

	funcs := make([]*schemav1.InMemoryFunction, len(p.Function))
	for i, v := range p.Function {
		funcs[i] = &schemav1.InMemoryFunction{
			Id:         v.Id,
			Name:       uint32(v.Name),
			SystemName: uint32(v.SystemName),
			Filename:   uint32(v.Filename),
			StartLine:  uint32(v.StartLine),
		}
	}

	if err := h.functions.ingest(ctx, funcs, rewrites); err != nil {
		return err
	}

	locs := make([]*schemav1.InMemoryLocation, len(p.Location))
	for i, v := range p.Location {
		x := &schemav1.InMemoryLocation{
			Id:        v.Id,
			Address:   v.Address,
			MappingId: uint32(v.MappingId),
			IsFolded:  v.IsFolded,
		}
		x.Line = make([]schemav1.InMemoryLine, len(v.Line))
		for j, line := range v.Line {
			x.Line[j] = schemav1.InMemoryLine{
				FunctionId: uint32(line.FunctionId),
				Line:       int32(line.Line),
			}
		}
		locs[i] = x
	}
	if err := h.locations.ingest(ctx, locs, rewrites); err != nil {
		return err
	}

	samplesPerType := h.convertSamples(ctx, rewrites, stacktracePartition, p.Sample)

	var profileIngested bool
	for idxType := range samplesPerType {
		samples := samplesPerType[idxType]

		profile := schemav1.InMemoryProfile{
			ID:                  id,
			SeriesFingerprint:   seriesFingerprints[idxType],
			StacktracePartition: stacktracePartition,
			Samples:             samples,
			DropFrames:          p.DropFrames,
			KeepFrames:          p.KeepFrames,
			TimeNanos:           p.TimeNanos,
			DurationNanos:       p.DurationNanos,
			Comments:            copySlice(p.Comment),
			DefaultSampleType:   p.DefaultSampleType,
		}

		profile.Samples = h.delta.computeDelta(profile, labels[idxType])
		profile.TotalValue = profile.Samples.Sum()

		if profile.Samples.Len() == 0 {
			level.Debug(h.logger).Log("msg", "profile is empty after delta computation", "metricName", metricName)
			continue
		}

		if err := h.profiles.ingest(ctx, []schemav1.InMemoryProfile{profile}, labels[idxType], metricName, rewrites); err != nil {
			return err
		}

		profileIngested = true
		h.totalSamples.Add(uint64(profile.Samples.Len()))
		h.metrics.sampleValuesIngested.WithLabelValues(metricName).Add(float64(profile.Samples.Len()))
		h.metrics.sampleValuesReceived.WithLabelValues(metricName).Add(float64(len(p.Sample)))
	}

	if !profileIngested {
		return nil
	}

	h.metaLock.Lock()
	v := model.TimeFromUnixNano(p.TimeNanos)
	if v < h.meta.MinTime {
		h.meta.MinTime = v
	}
	if v > h.meta.MaxTime {
		h.meta.MaxTime = v
	}
	h.metaLock.Unlock()

	return nil
}

// LabelValues returns the possible label values for a given label name.
func (h *Head) LabelValues(ctx context.Context, req *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	selectors, err := parseSelectors(req.Msg.Matchers)
	if err != nil {
		return nil, err
	}

	// shortcut to index when matcher match all
	if selectors.matchesAll() {
		values, err := h.profiles.index.ix.LabelValues(req.Msg.Name, nil)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(&typesv1.LabelValuesResponse{
			Names: values,
		}), nil
	}

	// aggregate all label values from series matching, when matchers are given.

	values := make(map[string]struct{})
	if err := h.forMatchingSelectors(selectors, func(lbs phlaremodel.Labels, fp model.Fingerprint) error {
		if v := lbs.Get(req.Msg.Name); v != "" {
			values[v] = struct{}{}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return connect.NewResponse(&typesv1.LabelValuesResponse{
		Names: lo.Keys(values),
	}), nil
}

// LabelNames returns the possible label values for a given label name.
func (h *Head) LabelNames(ctx context.Context, req *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	selectors, err := parseSelectors(req.Msg.Matchers)
	if err != nil {
		return nil, err
	}

	// shortcut to index when matcher match all
	if selectors.matchesAll() {
		values, err := h.profiles.index.ix.LabelNames(nil)
		if err != nil {
			return nil, err
		}
		sort.Strings(values)
		return connect.NewResponse(&typesv1.LabelNamesResponse{
			Names: values,
		}), nil
	}

	// aggregate all label values from series matching, when matchers are given.
	values := make(map[string]struct{})
	if err := h.forMatchingSelectors(selectors, func(lbs phlaremodel.Labels, fp model.Fingerprint) error {
		for _, lbl := range lbs {
			values[lbl.Name] = struct{}{}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return connect.NewResponse(&typesv1.LabelNamesResponse{
		Names: lo.Keys(values),
	}), nil
}

// ProfileTypes returns the possible profile types.
func (h *Head) ProfileTypes(ctx context.Context, req *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
	values, err := h.profiles.index.ix.LabelValues(phlaremodel.LabelNameProfileType, nil)
	if err != nil {
		return nil, err
	}
	sort.Strings(values)

	profileTypes := make([]*typesv1.ProfileType, len(values))
	for i, v := range values {
		tp, err := phlaremodel.ParseProfileTypeSelector(v)
		if err != nil {
			return nil, err
		}
		profileTypes[i] = tp
	}

	return connect.NewResponse(&ingestv1.ProfileTypesResponse{
		ProfileTypes: profileTypes,
	}), nil
}

func (h *Head) Bounds() (mint, maxt model.Time) {
	h.metaLock.RLock()
	defer h.metaLock.RUnlock()
	return h.meta.MinTime, h.meta.MaxTime
}

// Returns underlying queries, the queriers should be roughly ordered in TS increasing order
func (h *Head) Queriers() Queriers {
	h.profiles.rowsLock.RLock()
	defer h.profiles.rowsLock.RUnlock()

	queriers := make([]Querier, 0, len(h.profiles.rowGroups)+1)
	for idx := range h.profiles.rowGroups {
		queriers = append(queriers, &headOnDiskQuerier{
			head:        h,
			rowGroupIdx: idx,
		})
	}
	queriers = append(queriers, &headInMemoryQuerier{h})
	return queriers
}

// add the location IDs to the stacktraces
func (h *Head) resolveStacktraces(ctx context.Context, stacktracesByMapping stacktracesByMapping) *ingestv1.MergeProfilesStacktracesResult {
	sp, _ := opentracing.StartSpanFromContext(ctx, "resolveStacktraces - Head")
	defer sp.Finish()

	names := []string{}
	functions := map[uint32]int{}

	h.locations.lock.RLock()
	h.functions.lock.RLock()
	h.strings.lock.RLock()
	defer func() {
		h.locations.lock.RUnlock()
		h.functions.lock.RUnlock()
		h.strings.lock.RUnlock()
	}()

	sp.LogFields(otlog.String("msg", "building MergeProfilesStacktracesResult"))
	_ = stacktracesByMapping.ForEach(
		func(mapping uint64, stacktraceSamples stacktraceSampleMap) error {
			mp, ok := h.symbolDB.MappingReader(mapping)
			if !ok {
				return nil
			}
			resolver := mp.StacktraceResolver()
			defer resolver.Release()
			// sort the stacktrace IDs as expected by the resolver
			stacktraceIDs := stacktraceSamples.Ids()
			sort.Slice(stacktraceIDs, func(i, j int) bool {
				return stacktraceIDs[i] < stacktraceIDs[j]
			})
			return resolver.ResolveStacktraces(
				ctx,
				symdb.StacktraceInserterFn(
					func(stacktraceID uint32, locs []int32) {
						fnIds := make([]int32, 0, 2*len(locs))
						for _, loc := range locs {
							for _, line := range h.locations.slice[loc].Line {
								fnNameID := h.functions.slice[line.FunctionId].Name
								pos, ok := functions[fnNameID]
								if !ok {
									functions[fnNameID] = len(names)
									fnIds = append(fnIds, int32(len(names)))
									names = append(names, h.strings.slice[h.functions.slice[line.FunctionId].Name])
									continue
								}
								fnIds = append(fnIds, int32(pos))
							}
						}
						stacktraceSamples[stacktraceID].FunctionIds = fnIds
					},
				),
				stacktraceIDs,
			)
		},
	)

	return &ingestv1.MergeProfilesStacktracesResult{
		Stacktraces:   stacktracesByMapping.StacktraceSamples(),
		FunctionNames: names,
	}
}

func (h *Head) resolvePprof(ctx context.Context, stacktracesByMapping profileSampleByMapping) *profile.Profile {
	sp, _ := opentracing.StartSpanFromContext(ctx, "resolvePprof - Head")
	defer sp.Finish()

	locations := map[int32]*profile.Location{}
	functions := map[uint32]*profile.Function{}
	mappings := map[uint32]*profile.Mapping{}

	h.locations.lock.RLock()
	h.functions.lock.RLock()
	h.strings.lock.RLock()
	defer func() {
		h.locations.lock.RUnlock()
		h.functions.lock.RUnlock()
		h.strings.lock.RUnlock()
	}()

	// now add locationIDs and stacktraces
	_ = stacktracesByMapping.ForEach(
		func(mapping uint64, stacktraceSamples profileSampleMap) error {
			mp, ok := h.symbolDB.MappingReader(mapping)
			if !ok {
				return nil
			}
			resolver := mp.StacktraceResolver()
			defer resolver.Release()

			// sort the stacktrace IDs as expected by the resolver
			stacktraceIDs := stacktraceSamples.Ids()
			sort.Slice(stacktraceIDs, func(i, j int) bool {
				return stacktraceIDs[i] < stacktraceIDs[j]
			})

			return resolver.ResolveStacktraces(
				ctx,
				symdb.StacktraceInserterFn(
					func(stacktraceID uint32, locationIds []int32) {
						stacktraceLocations := make([]*profile.Location, len(locationIds))

						for i, locId := range locationIds {
							loc, ok := locations[locId]
							if !ok {
								locFound := h.locations.slice[locId]
								mapping, ok := mappings[locFound.MappingId]
								if !ok {
									mappingFound := h.mappings.slice[locFound.MappingId]
									mapping = &profile.Mapping{
										ID:              mappingFound.Id,
										Start:           mappingFound.MemoryStart,
										Limit:           mappingFound.MemoryLimit,
										Offset:          mappingFound.FileOffset,
										File:            h.strings.slice[mappingFound.Filename],
										BuildID:         h.strings.slice[mappingFound.BuildId],
										HasFunctions:    mappingFound.HasFunctions,
										HasFilenames:    mappingFound.HasFilenames,
										HasLineNumbers:  mappingFound.HasLineNumbers,
										HasInlineFrames: mappingFound.HasInlineFrames,
									}
									mappings[locFound.MappingId] = mapping
								}
								loc = &profile.Location{
									ID:       locFound.Id,
									Address:  locFound.Address,
									IsFolded: locFound.IsFolded,
									Mapping:  mapping,
									Line:     make([]profile.Line, len(locFound.Line)),
								}
								for i, line := range locFound.Line {
									fn, ok := functions[line.FunctionId]
									if !ok {
										fnFound := h.functions.slice[line.FunctionId]
										fn = &profile.Function{
											ID:         fnFound.Id,
											Name:       h.strings.slice[fnFound.Name],
											SystemName: h.strings.slice[fnFound.SystemName],
											Filename:   h.strings.slice[fnFound.Filename],
											StartLine:  int64(fnFound.StartLine),
										}
										functions[line.FunctionId] = fn
									}
									loc.Line[i] = profile.Line{
										Line:     int64(line.Line),
										Function: fn,
									}
								}
								locations[locId] = loc
							}
							stacktraceLocations[i] = loc
						}
						stacktraceSamples[stacktraceID].Location = stacktraceLocations
					},
				),
				stacktraceIDs,
			)
		},
	)

	result := &profile.Profile{
		Sample:   stacktracesByMapping.StacktraceSamples(),
		Location: lo.Values(locations),
		Function: lo.Values(functions),
		Mapping:  lo.Values(mappings),
	}
	normalizeProfileIds(result)
	return result
}

func normalizeProfileIds(p *profile.Profile) {
	// normalize IDs
	for i, l := range p.Location {
		l.ID = uint64(i) + 1
	}
	for i, f := range p.Function {
		f.ID = uint64(i) + 1
	}
	for i, m := range p.Mapping {
		m.ID = uint64(i) + 1
	}
}

func (h *Head) Sort(in []Profile) []Profile {
	return in
}

type ProfileSelectorIterator struct {
	batch   chan []Profile
	current iter.Iterator[Profile]
	once    sync.Once
}

func NewProfileSelectorIterator() *ProfileSelectorIterator {
	return &ProfileSelectorIterator{
		batch: make(chan []Profile, 1),
	}
}

func (it *ProfileSelectorIterator) Push(batch []Profile) {
	if len(batch) == 0 {
		return
	}
	it.batch <- batch
}

func (it *ProfileSelectorIterator) Next() bool {
	if it.current == nil {
		batch, ok := <-it.batch
		if !ok {
			return false
		}
		it.current = iter.NewSliceIterator(batch)
	}
	if !it.current.Next() {
		it.current = nil
		return it.Next()
	}
	return true
}

func (it *ProfileSelectorIterator) At() Profile {
	if it.current == nil {
		return ProfileWithLabels{}
	}
	return it.current.At()
}

func (it *ProfileSelectorIterator) Close() error {
	it.once.Do(func() {
		close(it.batch)
	})
	return nil
}

func (it *ProfileSelectorIterator) Err() error {
	return nil
}

// selectors are composed of any amount of selectors which are ORed
type selectors [][]*labels.Matcher

func parseSelectors(selectorStrings []string) (selectors, error) {
	sels := make([][]*labels.Matcher, 0, len(selectorStrings))
	for _, m := range selectorStrings {
		s, err := parser.ParseMetricSelector(m)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("failed to parse label selector: %v", err))
		}
		sels = append(sels, s)
	}

	return sels, nil
}

func (sels selectors) matchesAll() bool {
	if len(sels) == 0 {
		return true
	}

	for _, sel := range sels {
		if len(sel) == 0 {
			return true
		}
	}

	return false
}

func (h *Head) forMatchingSelectors(sels selectors, fn func(lbs phlaremodel.Labels, fp model.Fingerprint) error) error {
	if sels.matchesAll() {
		return h.profiles.index.forMatchingLabels(nil, fn)
	}

	for _, sel := range sels {
		if err := h.profiles.index.forMatchingLabels(sel, fn); err != nil {
			return err
		}
	}

	return nil
}

func (h *Head) Series(ctx context.Context, req *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error) {
	selectors, err := parseSelectors(req.Msg.Matchers)
	if err != nil {
		return nil, err
	}
	response := &ingestv1.SeriesResponse{}
	uniqu := map[model.Fingerprint]struct{}{}
	if err := h.forMatchingSelectors(selectors, func(lbs phlaremodel.Labels, fp model.Fingerprint) error {
		if _, ok := uniqu[fp]; ok {
			return nil
		}
		uniqu[fp] = struct{}{}
		response.LabelsSet = append(response.LabelsSet, &typesv1.Labels{Labels: lbs})
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(response.LabelsSet, func(i, j int) bool {
		return phlaremodel.CompareLabelPairs(response.LabelsSet[i].Labels, response.LabelsSet[j].Labels) < 0
	})
	return connect.NewResponse(response), nil
}

// Flush closes the head and writes data to disk. No ingestion requests should
// be made concurrently with the call, or after it returns.
// The call is thread-safe for reads.
func (h *Head) Flush(ctx context.Context) error {
	close(h.stopCh)
	h.wg.Wait()
	start := time.Now()
	defer func() {
		h.metrics.flushedBlockDurationSeconds.Observe(time.Since(start).Seconds())
	}()
	if err := h.flush(ctx); err != nil {
		h.metrics.flushedBlocks.WithLabelValues("failed").Inc()
		return err
	}
	h.metrics.flushedBlocks.WithLabelValues("success").Inc()
	return nil
}

func (h *Head) flush(ctx context.Context) error {
	// Ensure all the in-flight ingestion requests have finished.
	// It must be guaranteed that no new inserts will happen
	// after the call start.
	h.inFlightProfiles.Wait()
	if len(h.profiles.slice) == 0 {
		level.Info(h.logger).Log("msg", "head empty - no block written")
		return os.RemoveAll(h.headPath)
	}

	files := make([]block.File, len(h.tables)+1)

	for idx, t := range h.tables {
		numRows, numRowGroups, err := t.Flush(ctx)
		if err != nil {
			return errors.Wrapf(err, "flushing of table %s", t.Name())
		}
		h.metrics.rowsWritten.WithLabelValues(t.Name()).Add(float64(numRows))
		files[idx+1].Parquet = &block.ParquetFile{
			NumRowGroups: numRowGroups,
			NumRows:      numRows,
		}
	}

	if err := h.symbolDB.Flush(); err != nil {
		return errors.Wrap(err, "flushing symbol database")
	}

	// get stats of index
	indexPath := filepath.Join(h.headPath, block.IndexFilename)
	files[0].RelPath = block.IndexFilename
	h.meta.Stats.NumSeries = uint64(h.profiles.index.totalSeries.Load())
	h.metrics.flushedBlockSeries.Observe(float64(h.meta.Stats.NumSeries))
	files[0].TSDB = &block.TSDBFile{
		NumSeries: h.meta.Stats.NumSeries,
	}
	// add index file size
	if stat, err := os.Stat(indexPath); err == nil {
		files[0].SizeBytes = uint64(stat.Size())
		h.metrics.flushedFileSizeBytes.WithLabelValues("tsdb").Observe(float64(files[0].SizeBytes))
	}
	totalSize := files[0].SizeBytes

	for idx, t := range h.tables {
		if err := t.Close(); err != nil {
			return errors.Wrapf(err, "closing of table %s", t.Name())
		}

		// add file size
		files[idx+1].RelPath = t.Name() + block.ParquetSuffix
		if stat, err := os.Stat(filepath.Join(h.headPath, files[idx+1].RelPath)); err == nil {
			files[idx+1].SizeBytes = uint64(stat.Size())
			h.metrics.flushedFileSizeBytes.WithLabelValues(t.Name()).Observe(float64(files[idx+1].SizeBytes))
			totalSize += files[idx+1].SizeBytes
		}
	}

	// add total size symdb
	symbDBFiles, error := h.SymDBFiles()
	if error != nil {
		return error
	}

	for _, file := range symbDBFiles {
		files = append(files, file)
		h.metrics.flushedFileSizeBytes.WithLabelValues(file.RelPath).Observe(float64(file.SizeBytes))
		totalSize += file.SizeBytes
	}

	h.metrics.flushedBlockSizeBytes.Observe(float64(totalSize))
	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})
	h.meta.Files = files
	h.meta.Stats.NumProfiles = uint64(h.profiles.index.totalProfiles.Load())
	h.meta.Stats.NumSamples = h.totalSamples.Load()
	h.metrics.flushedBlockSamples.Observe(float64(h.meta.Stats.NumSamples))
	h.metrics.flusehdBlockProfiles.Observe(float64(h.meta.Stats.NumProfiles))

	if _, err := h.meta.WriteToFile(h.logger, h.headPath); err != nil {
		return err
	}
	h.metrics.blockDurationSeconds.Observe(h.meta.MaxTime.Sub(h.meta.MinTime).Seconds())
	return nil
}

// SymDBFiles lists files in symdb folder
func (h *Head) SymDBFiles() ([]block.File, error) {
	files, err := os.ReadDir(filepath.Join(h.headPath, symdb.DefaultDirName))
	if err != nil {
		return nil, err
	}
	result := make([]block.File, len(files))
	for idx, f := range files {
		if f.IsDir() {
			continue
		}
		result[idx].RelPath = filepath.Join(symdb.DefaultDirName, f.Name())
		info, err := f.Info()
		if err != nil {
			return nil, err
		}
		result[idx].SizeBytes = uint64(info.Size())
	}
	return result, nil
}

// Move moves the head directory to local blocks. The call is not thread-safe:
// no concurrent reads and writes are allowed.
//
// After the call, head in-memory representation is not valid and should not
// be accessed for querying.
func (h *Head) Move() error {
	// Remove intermediate row groups before the move as they are still
	// referencing files on the disk.
	if err := h.profiles.DeleteRowGroups(); err != nil {
		return err
	}

	// move block to the local directory
	if err := os.MkdirAll(filepath.Dir(h.localPath), defaultFolderMode); err != nil {
		return err
	}
	if err := fileutil.Rename(h.headPath, h.localPath); err != nil {
		return err
	}

	level.Info(h.logger).Log("msg", "head successfully written to block", "block_path", h.localPath)
	return nil
}
