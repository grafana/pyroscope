package phlaredb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/status"
	"github.com/google/pprof/profile"
	"github.com/google/uuid"
	"github.com/grafana/dskit/multierror"
	"github.com/opentracing/opentracing-go"
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
	"github.com/grafana/phlare/pkg/slices"
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

type Models interface {
	*schemav1.Profile | *schemav1.Stacktrace | *profilev1.Location | *profilev1.Mapping | *profilev1.Function | string | *schemav1.StoredString
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
	locations   idConversionTable
	stacktraces idConversionTable
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

	flushCh chan struct{} // this channel is closed once the Head should be flushed, should be used externally

	flushForcedTimer *time.Timer // this timer will phlare after the maximum

	metaLock sync.RWMutex
	meta     *block.Meta

	parquetConfig   *ParquetConfig
	strings         deduplicatingSlice[string, string, *stringsHelper, *schemav1.StringPersister]
	mappings        deduplicatingSlice[*profilev1.Mapping, mappingsKey, *mappingsHelper, *schemav1.MappingPersister]
	functions       deduplicatingSlice[*profilev1.Function, functionsKey, *functionsHelper, *schemav1.FunctionPersister]
	locations       deduplicatingSlice[*profilev1.Location, locationsKey, *locationsHelper, *schemav1.LocationPersister]
	stacktraces     deduplicatingSlice[*schemav1.Stacktrace, stacktracesKey, *stacktracesHelper, *schemav1.StacktracePersister] // a stacktrace is a slice of location ids
	profiles        *profileStore
	totalSamples    *atomic.Uint64
	tables          []Table
	delta           *deltaProfiles
	pprofLabelCache labelCache

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
		&h.stacktraces,
		h.profiles,
	}
	for _, t := range h.tables {
		if err := t.Init(h.headPath, h.parquetConfig, h.metrics); err != nil {
			return nil, err
		}
	}

	h.delta = newDeltaProfiles()

	h.pprofLabelCache.init()

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

	return size
}

func (h *Head) Size() uint64 {
	var size uint64
	// TODO: Estimate size of TSDB index
	for _, t := range h.tables {
		size += t.Size()
	}

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
			level.Debug(h.logger).Log("msg", "max block duration reached, flush to disk")
			close(h.flushCh)
			return
		case <-tick.C:
			if currentSize := h.Size(); currentSize > h.parquetConfig.MaxBlockBytes {
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

func (h *Head) convertSamples(ctx context.Context, r *rewriter, in []*profilev1.Sample) ([][]*schemav1.Sample, error) {
	if len(in) == 0 {
		return nil, nil
	}

	// populate output
	var (
		out         = make([][]*schemav1.Sample, len(in[0].Value))
		stacktraces = make([]*schemav1.Stacktrace, len(in))
	)
	for idxType := range out {
		out[idxType] = make([]*schemav1.Sample, len(in))
	}

	for idxSample := range in {
		// populate samples
		labels := h.pprofLabelCache.rewriteLabels(r.strings, in[idxSample].Label)
		for idxType := range out {
			out[idxType][idxSample] = &schemav1.Sample{
				Value:  in[idxSample].Value[idxType],
				Labels: labels,
			}
		}

		// build full stack traces
		stacktraces[idxSample] = &schemav1.Stacktrace{
			// no copySlice necessary at this point,stacktracesHelper.clone
			// will copy it, if it is required to be retained.
			LocationIDs: in[idxSample].LocationId,
		}
	}

	// ingest stacktraces
	if err := h.stacktraces.ingest(ctx, stacktraces, r); err != nil {
		return nil, err
	}

	// reference stacktraces
	for idxType := range out {
		for idxSample := range out[idxType] {
			out[idxType][idxSample].StacktraceID = uint64(r.stacktraces[int64(idxSample)])
		}
	}

	return out, nil
}

func (h *Head) Ingest(ctx context.Context, p *profilev1.Profile, id uuid.UUID, externalLabels ...*typesv1.LabelPair) error {
	labels, seriesFingerprints := labelsForProfile(p, externalLabels...)

	for i, fp := range seriesFingerprints {
		if err := h.limiter.AllowProfile(fp, labels[i], p.TimeNanos); err != nil {
			return err
		}
	}

	metricName := phlaremodel.Labels(externalLabels).Get(model.MetricNameLabel)

	// create a rewriter state
	rewrites := &rewriter{}

	if err := h.strings.ingest(ctx, p.StringTable, rewrites); err != nil {
		return err
	}

	if err := h.mappings.ingest(ctx, p.Mapping, rewrites); err != nil {
		return err
	}

	if err := h.functions.ingest(ctx, p.Function, rewrites); err != nil {
		return err
	}

	if err := h.locations.ingest(ctx, p.Location, rewrites); err != nil {
		return err
	}

	samplesPerType, err := h.convertSamples(ctx, rewrites, p.Sample)
	if err != nil {
		return err
	}

	var profileIngested bool
	for idxType := range samplesPerType {
		samples := samplesPerType[idxType]
		// Sort samples per stacktraceID and aggregate duplicate stacktraceIDs into
		// a single value to make sure we won't have any duplicates, as this is not recognized as part of the delta calculation.
		sort.Slice(samples, func(i, j int) bool {
			return samples[i].StacktraceID > samples[j].StacktraceID
		})
		total := len(samples)
		samples = slices.RemoveInPlace(samples, func(s *schemav1.Sample, i int) bool {
			if s.Value == 0 {
				return true
			}
			if i < len(p.Sample)-1 && s.StacktraceID == samples[i+1].StacktraceID {
				samples[i+1].Value += s.Value
				// TODO: Currently we're not aggregating labels, and we should probably decide what to do with them in this case.
				return true
			}
			return false
		})
		if total != len(samples) {
			// copy samples if there are less than received to avoid retaining memory.
			samples = copySlice(samples)
		}
		profile := &schemav1.Profile{
			ID:                id,
			SeriesFingerprint: seriesFingerprints[idxType],
			Samples:           samples,
			DropFrames:        p.DropFrames,
			KeepFrames:        p.KeepFrames,
			TimeNanos:         p.TimeNanos,
			DurationNanos:     p.DurationNanos,
			Comments:          copySlice(p.Comment),
			DefaultSampleType: p.DefaultSampleType,
		}

		profile = h.delta.computeDelta(profile, labels[idxType])

		if profile == nil {
			level.Debug(h.logger).Log("msg", "profile is empty after delta computation", "metricName", metricName)
			continue
		}

		if err := h.profiles.ingest(ctx, []*schemav1.Profile{profile}, labels[idxType], metricName, rewrites); err != nil {
			return err
		}

		profileIngested = true
		h.totalSamples.Add(uint64(len(profile.Samples)))
		h.metrics.sampleValuesIngested.WithLabelValues(metricName).Add(float64(len(profile.Samples)))
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

func labelsForProfile(p *profilev1.Profile, externalLabels ...*typesv1.LabelPair) ([]phlaremodel.Labels, []model.Fingerprint) {
	// build label set per sample type before references are rewritten
	var (
		sb                                             strings.Builder
		lbls                                           = phlaremodel.NewLabelsBuilder(externalLabels)
		sampleType, sampleUnit, periodType, periodUnit string
		metricName                                     = phlaremodel.Labels(externalLabels).Get(model.MetricNameLabel)
	)

	// set common labels
	if p.PeriodType != nil {
		periodType = p.StringTable[p.PeriodType.Type]
		lbls.Set(phlaremodel.LabelNamePeriodType, periodType)
		periodUnit = p.StringTable[p.PeriodType.Unit]
		lbls.Set(phlaremodel.LabelNamePeriodUnit, periodUnit)
	}

	profilesLabels := make([]phlaremodel.Labels, len(p.SampleType))
	seriesRefs := make([]model.Fingerprint, len(p.SampleType))
	for pos := range p.SampleType {
		sampleType = p.StringTable[p.SampleType[pos].Type]
		lbls.Set(phlaremodel.LabelNameType, sampleType)
		sampleUnit = p.StringTable[p.SampleType[pos].Unit]
		lbls.Set(phlaremodel.LabelNameUnit, sampleUnit)

		sb.Reset()
		_, _ = sb.WriteString(metricName)
		_, _ = sb.WriteRune(':')
		_, _ = sb.WriteString(sampleType)
		_, _ = sb.WriteRune(':')
		_, _ = sb.WriteString(sampleUnit)
		_, _ = sb.WriteRune(':')
		_, _ = sb.WriteString(periodType)
		_, _ = sb.WriteRune(':')
		_, _ = sb.WriteString(periodUnit)
		t := sb.String()
		lbls.Set(phlaremodel.LabelNameProfileType, t)
		lbs := lbls.Labels().Clone()
		profilesLabels[pos] = lbs
		seriesRefs[pos] = model.Fingerprint(lbs.Hash())

	}
	return profilesLabels, seriesRefs
}

// LabelValues returns the possible label values for a given label name.
func (h *Head) LabelValues(ctx context.Context, req *connect.Request[ingestv1.LabelValuesRequest]) (*connect.Response[ingestv1.LabelValuesResponse], error) {
	values, err := h.profiles.index.ix.LabelValues(req.Msg.Name, nil)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&ingestv1.LabelValuesResponse{
		Names: values,
	}), nil
}

// LabelNames returns the possible label values for a given label name.
func (h *Head) LabelNames(ctx context.Context, req *connect.Request[ingestv1.LabelNamesRequest]) (*connect.Response[ingestv1.LabelNamesResponse], error) {
	values, err := h.profiles.index.ix.LabelNames(nil)
	if err != nil {
		return nil, err
	}
	sort.Strings(values)
	return connect.NewResponse(&ingestv1.LabelNamesResponse{
		Names: values,
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

func (h *Head) InRange(start, end model.Time) bool {
	h.metaLock.RLock()
	b := &minMax{
		min: h.meta.MinTime,
		max: h.meta.MaxTime,
	}
	h.metaLock.RUnlock()
	return b.InRange(start, end)
}

// Returns underlying queries, the queriers should be roughly ordered in TS increasing order
func (h *Head) Queriers() Queriers {
	h.profiles.lock.RLock()
	defer h.profiles.lock.RUnlock()

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
func (h *Head) resolveStacktraces(ctx context.Context, stacktraceSamples stacktraceSampleMap) *ingestv1.MergeProfilesStacktracesResult {
	sp, _ := opentracing.StartSpanFromContext(ctx, "resolveStacktraces - Head")
	defer sp.Finish()

	names := []string{}
	functions := map[int64]int{}

	h.stacktraces.lock.RLock()
	h.locations.lock.RLock()
	h.functions.lock.RLock()
	h.strings.lock.RLock()
	defer func() {
		h.stacktraces.lock.RUnlock()
		h.locations.lock.RUnlock()
		h.functions.lock.RUnlock()
		h.strings.lock.RUnlock()
	}()

	for stacktraceID := range stacktraceSamples {
		locs := h.stacktraces.slice[stacktraceID].LocationIDs
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
	}

	return &ingestv1.MergeProfilesStacktracesResult{
		Stacktraces:   lo.Values(stacktraceSamples),
		FunctionNames: names,
	}
}

func (h *Head) resolvePprof(ctx context.Context, stacktraceSamples profileSampleMap) *profile.Profile {
	sp, _ := opentracing.StartSpanFromContext(ctx, "resolvePprof - Head")
	defer sp.Finish()

	locations := map[uint64]*profile.Location{}
	functions := map[uint64]*profile.Function{}
	mappings := map[uint64]*profile.Mapping{}

	h.stacktraces.lock.RLock()
	h.locations.lock.RLock()
	h.functions.lock.RLock()
	h.strings.lock.RLock()
	defer func() {
		h.stacktraces.lock.RUnlock()
		h.locations.lock.RUnlock()
		h.functions.lock.RUnlock()
		h.strings.lock.RUnlock()
	}()

	// now add locationIDs and stacktraces
	for stacktraceID := range stacktraceSamples {
		locationIds := h.stacktraces.slice[stacktraceID].LocationIDs
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
							StartLine:  fnFound.StartLine,
						}
						functions[line.FunctionId] = fn
					}
					loc.Line[i] = profile.Line{
						Line:     line.Line,
						Function: fn,
					}
				}
				locations[locId] = loc
			}
			stacktraceLocations[i] = loc
		}
		stacktraceSamples[stacktraceID].Location = stacktraceLocations
	}

	result := &profile.Profile{
		Sample:   lo.Values(stacktraceSamples),
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

func (h *Head) Series(ctx context.Context, req *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error) {
	selectors := make([][]*labels.Matcher, 0, len(req.Msg.Matchers))
	for _, m := range req.Msg.Matchers {
		s, err := parser.ParseMetricSelector(m)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "failed to label selector")
		}
		selectors = append(selectors, s)
	}
	response := &ingestv1.SeriesResponse{}
	uniqu := map[model.Fingerprint]struct{}{}
	for _, selector := range selectors {
		if err := h.profiles.index.forMatchingLabels(selector, func(lbs phlaremodel.Labels, fp model.Fingerprint) error {
			if _, ok := uniqu[fp]; ok {
				return nil
			}
			uniqu[fp] = struct{}{}
			response.LabelsSet = append(response.LabelsSet, &typesv1.Labels{Labels: lbs})
			return nil
		}); err != nil {
			return nil, err
		}
	}
	sort.Slice(response.LabelsSet, func(i, j int) bool {
		return phlaremodel.CompareLabelPairs(response.LabelsSet[i].Labels, response.LabelsSet[j].Labels) < 0
	})
	return connect.NewResponse(response), nil
}

// Closes closes the head
func (h *Head) Close() error {
	close(h.stopCh)

	var merr multierror.MultiError
	for _, t := range h.tables {
		merr.Add(t.Close())
	}

	h.wg.Wait()
	return merr.Err()
}

// Flush closes the head and writes data to disk
func (h *Head) Flush(ctx context.Context) error {
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
	if h.profiles.empty() {
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
