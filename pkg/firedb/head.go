package firedb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/status"
	"github.com/google/uuid"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"go.uber.org/atomic"
	"google.golang.org/grpc/codes"

	"github.com/grafana/fire/pkg/firedb/block"
	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	firemodel "github.com/grafana/fire/pkg/model"
)

func copySlice[T any](in []T) []T {
	out := make([]T, len(in))
	copy(out, in)
	return out
}

type idConversionTable map[int64]int64

func (t idConversionTable) rewrite(idx *int64) {
	pos := *idx
	var ok bool
	*idx, ok = t[pos]
	if !ok {
		panic(fmt.Sprintf("unable to rewrite index %d", pos))
	}
}

func (t idConversionTable) rewriteUint64(idx *uint64) {
	pos := *idx
	v, ok := t[int64(pos)]
	if !ok {
		panic(fmt.Sprintf("unable to rewrite index %d", pos))
	}
	*idx = uint64(v)
}

type Models interface {
	*schemav1.Profile | *schemav1.Stacktrace | *profilev1.Location | *profilev1.Mapping | *profilev1.Function | string
}

// rewriter contains slices to rewrite the per profile reference into per head references.
type rewriter struct {
	strings     stringConversionTable
	functions   idConversionTable
	mappings    idConversionTable
	locations   idConversionTable
	stacktraces idConversionTable
}

type Helper[M Models, K comparable] interface {
	key(M) K
	addToRewriter(*rewriter, idConversionTable)
	rewrite(*rewriter, M) error
	// some Models contain their own IDs within the struct, this allows to set them and keep track of the preexisting ID. It should return the oldID that is supposed to be rewritten.
	setID(existingSliceID uint64, newID uint64, element M) uint64

	// size returns a (rough estimation) of the size of a single element M
	size(M) uint64

	// clone copies parts that are not optimally sized from protobuf parsing
	clone(M) M
}

type Table interface {
	Name() string
	Size() uint64
	Init(path string) error
	Flush() (numRows uint64, numRowGroups uint64, err error)
	Close() error
}

func HeadWithRegistry(reg prometheus.Registerer) HeadOption {
	return func(h *Head) {
		h.metrics = newHeadMetrics(reg).setHead(h)
	}
}

func headWithMetrics(m *headMetrics) HeadOption {
	return func(h *Head) {
		h.metrics = m.setHead(h)
	}
}

func HeadWithLogger(l log.Logger) HeadOption {
	return func(h *Head) {
		h.logger = l
	}
}

type HeadOption func(*Head)

type Head struct {
	logger  log.Logger
	metrics *headMetrics

	headPath  string // path while block is actively appended to
	localPath string // path once block has been cut

	metaLock sync.RWMutex
	meta     *block.Meta

	index           *profilesIndex
	strings         deduplicatingSlice[string, string, *stringsHelper, *schemav1.StringPersister]
	mappings        deduplicatingSlice[*profilev1.Mapping, mappingsKey, *mappingsHelper, *schemav1.MappingPersister]
	functions       deduplicatingSlice[*profilev1.Function, functionsKey, *functionsHelper, *schemav1.FunctionPersister]
	locations       deduplicatingSlice[*profilev1.Location, locationsKey, *locationsHelper, *schemav1.LocationPersister]
	stacktraces     deduplicatingSlice[*schemav1.Stacktrace, stacktracesKey, *stacktracesHelper, *schemav1.StacktracePersister] // a stacktrace is a slice of location ids
	profiles        deduplicatingSlice[*schemav1.Profile, noKey, *profilesHelper, *schemav1.ProfilePersister]
	totalSamples    *atomic.Uint64
	tables          []Table
	delta           *deltaProfiles
	pprofLabelCache labelCache
}

const (
	pathHead          = "head"
	pathLocal         = "local"
	defaultFolderMode = 0o755
)

func NewHead(dataPath string, opts ...HeadOption) (*Head, error) {
	h := &Head{
		meta:         block.NewMeta(),
		totalSamples: atomic.NewUint64(0),
	}
	h.headPath = filepath.Join(dataPath, pathHead, h.meta.ULID.String())
	h.localPath = filepath.Join(dataPath, pathLocal, h.meta.ULID.String())

	// execute options
	for _, o := range opts {
		o(h)
	}

	// setup fall backs
	if h.logger == nil {
		h.logger = log.NewNopLogger()
	}
	if h.metrics == nil {
		h.metrics = newHeadMetrics(nil).setHead(h)
	}

	if err := os.MkdirAll(h.headPath, defaultFolderMode); err != nil {
		return nil, err
	}

	h.tables = []Table{
		&h.strings,
		&h.mappings,
		&h.functions,
		&h.locations,
		&h.stacktraces,
		&h.profiles,
	}
	for _, t := range h.tables {
		if err := t.Init(h.headPath); err != nil {
			return nil, err
		}
	}

	index, err := newProfileIndex(32, h.metrics)
	if err != nil {
		return nil, err
	}
	h.index = index
	h.delta = newDeltaProfiles()

	h.pprofLabelCache.init()
	return h, nil
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

func (h *Head) Ingest(ctx context.Context, p *profilev1.Profile, id uuid.UUID, externalLabels ...*commonv1.LabelPair) error {
	metricName := firemodel.Labels(externalLabels).Get(model.MetricNameLabel)
	labels, seriesFingerprints := labelsForProfile(p, externalLabels...)

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
		profile := &schemav1.Profile{
			ID:                id,
			SeriesFingerprint: seriesFingerprints[idxType],
			Samples:           samplesPerType[idxType],
			DropFrames:        p.DropFrames,
			KeepFrames:        p.KeepFrames,
			TimeNanos:         p.TimeNanos,
			DurationNanos:     p.DurationNanos,
			Comments:          copySlice(p.Comment),
			DefaultSampleType: p.DefaultSampleType,
		}

		profile = h.delta.computeDelta(profile, labels[idxType])

		if profile == nil {
			continue
		}

		if err := h.profiles.ingest(ctx, []*schemav1.Profile{profile}, rewrites); err != nil {
			return err
		}

		h.index.Add(profile, labels[idxType], metricName)

		profileIngested = true
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

	samplesInProfile := len(p.Sample) * len(labels)
	h.totalSamples.Add(sampleSize)
	h.metrics.sampleValuesIngested.WithLabelValues(metricName).Add(float64(samplesInProfile))
	h.metrics.sampleValuesReceived.WithLabelValues(metricName).Add(float64(len(p.Sample) * len(labels)))

	return nil
}

func labelsForProfile(p *profilev1.Profile, externalLabels ...*commonv1.LabelPair) ([]firemodel.Labels, []model.Fingerprint) {
	// build label set per sample type before references are rewritten
	var (
		sb                                             strings.Builder
		lbls                                           = firemodel.NewLabelsBuilder(externalLabels)
		sampleType, sampleUnit, periodType, periodUnit string
		metricName                                     = firemodel.Labels(externalLabels).Get(model.MetricNameLabel)
	)

	// set common labels
	if p.PeriodType != nil {
		periodType = p.StringTable[p.PeriodType.Type]
		lbls.Set(firemodel.LabelNamePeriodType, periodType)
		periodUnit = p.StringTable[p.PeriodType.Unit]
		lbls.Set(firemodel.LabelNamePeriodUnit, periodUnit)
	}

	profilesLabels := make([]firemodel.Labels, len(p.SampleType))
	seriesRefs := make([]model.Fingerprint, len(p.SampleType))
	for pos := range p.SampleType {
		sampleType = p.StringTable[p.SampleType[pos].Type]
		lbls.Set(firemodel.LabelNameType, sampleType)
		sampleUnit = p.StringTable[p.SampleType[pos].Unit]
		lbls.Set(firemodel.LabelNameUnit, sampleUnit)

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
		lbls.Set(firemodel.LabelNameProfileType, t)
		lbs := lbls.Labels().Clone()
		profilesLabels[pos] = lbs
		seriesRefs[pos] = model.Fingerprint(lbs.Hash())

	}
	return profilesLabels, seriesRefs
}

// LabelValues returns the possible label values for a given label name.
func (h *Head) LabelValues(ctx context.Context, req *connect.Request[ingestv1.LabelValuesRequest]) (*connect.Response[ingestv1.LabelValuesResponse], error) {
	values, err := h.index.ix.LabelValues(req.Msg.Name, nil)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&ingestv1.LabelValuesResponse{
		Names: values,
	}), nil
}

// LabelValues returns the possible label values for a given label name.
func (h *Head) LabelNames(ctx context.Context, req *connect.Request[ingestv1.LabelNamesRequest]) (*connect.Response[ingestv1.LabelNamesResponse], error) {
	values, err := h.index.ix.LabelNames(nil)
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
	values, err := h.index.ix.LabelValues(firemodel.LabelNameProfileType, nil)
	if err != nil {
		return nil, err
	}
	sort.Strings(values)

	profileTypes := make([]*commonv1.ProfileType, len(values))
	for i, v := range values {
		tp, err := firemodel.ParseProfileTypeSelector(v)
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

func (h *Head) SelectProfiles(ctx context.Context, req *connect.Request[ingestv1.SelectProfilesRequest]) (*connect.Response[ingestv1.SelectProfilesResponse], error) {
	var (
		totalSamples   int64
		totalLocations int64
		totalProfiles  int64
	)
	// nolint:ineffassign
	// we might use ctx later.
	sp, ctx := opentracing.StartSpanFromContext(ctx, "Head - SelectProfiles")
	defer func() {
		sp.LogFields(
			otlog.Int64("total_samples", totalSamples),
			otlog.Int64("total_locations", totalLocations),
			otlog.Int64("total_profiles", totalProfiles),
		)
		sp.Finish()
	}()

	selectors, err := parser.ParseMetricSelector(req.Msg.LabelSelector)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to parse label selectors: "+err.Error())
	}
	selectors = append(selectors, firemodel.SelectorFromProfileType(req.Msg.Type))

	result := []*ingestv1.Profile{}
	names := []string{}
	stackTraces := map[uint64][]int32{}
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

	err = h.index.forMatchingProfiles(selectors, func(lbs firemodel.Labels, _ model.Fingerprint, profile *schemav1.Profile) error {
		ts := int64(model.TimeFromUnixNano(profile.TimeNanos))
		// if the timestamp is not matching we skip this profile.
		if req.Msg.Start > ts || ts > req.Msg.End {
			return nil
		}
		totalProfiles++
		p := &ingestv1.Profile{
			ID:          profile.ID.String(),
			Type:        req.Msg.Type,
			Labels:      lbs,
			Timestamp:   ts,
			Stacktraces: make([]*ingestv1.StacktraceSample, 0, len(profile.Samples)),
		}
		totalSamples += int64(len(profile.Samples))
		for _, s := range profile.Samples {
			if s.Value == 0 {
				totalSamples--
				continue
			}
			stackTracesIds, ok := stackTraces[s.StacktraceID]
			if !ok {
				locs := h.stacktraces.slice[s.StacktraceID].LocationIDs
				totalLocations += int64(len(locs))
				stackTracesIds = make([]int32, 0, 2*len(locs))
				for _, loc := range locs {
					for _, line := range h.locations.slice[loc].Line {
						fnNameID := h.functions.slice[line.FunctionId].Name
						pos, ok := functions[fnNameID]
						if !ok {
							functions[fnNameID] = len(names)
							stackTracesIds = append(stackTracesIds, int32(len(names)))
							names = append(names, h.strings.slice[h.functions.slice[line.FunctionId].Name])
							continue
						}
						stackTracesIds = append(stackTracesIds, int32(pos))
					}
				}
				stackTraces[s.StacktraceID] = stackTracesIds
			}

			p.Stacktraces = append(p.Stacktraces, &ingestv1.StacktraceSample{
				Value:       s.Value,
				FunctionIds: stackTracesIds,
			})
		}
		if len(p.Stacktraces) > 0 {
			result = append(result, p)
		}
		return nil
	})
	sort.Slice(result, func(i, j int) bool {
		return firemodel.CompareProfile(result[i], result[j]) < 0
	})
	return connect.NewResponse(&ingestv1.SelectProfilesResponse{
		Profiles:      result,
		FunctionNames: names,
	}), err
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
		if err := h.index.forMatchingLabels(selector, func(lbs firemodel.Labels, fp model.Fingerprint) error {
			if _, ok := uniqu[fp]; ok {
				return nil
			}
			uniqu[fp] = struct{}{}
			response.LabelsSet = append(response.LabelsSet, &commonv1.Labels{Labels: lbs})
			return nil
		}); err != nil {
			return nil, err
		}
	}
	sort.Slice(response.LabelsSet, func(i, j int) bool {
		return firemodel.CompareLabelPairs(response.LabelsSet[i].Labels, response.LabelsSet[j].Labels) < 0
	})
	return connect.NewResponse(response), nil
}

func (h *Head) Flush(ctx context.Context) error {
	if len(h.profiles.slice) == 0 {
		level.Info(h.logger).Log("msg", "head empty - no block written")
		return os.RemoveAll(h.headPath)
	}

	files := make([]block.File, len(h.tables)+1)

	// write index
	indexPath := filepath.Join(h.headPath, block.IndexFilename)
	if err := h.index.WriteTo(ctx, indexPath); err != nil {
		return errors.Wrap(err, "flushing of index")
	}
	files[0].RelPath = block.IndexFilename
	h.meta.Stats.NumSeries = uint64(h.index.totalSeries.Load())
	files[0].TSDB = &block.TSDBFile{
		NumSeries: h.meta.Stats.NumSeries,
	}

	// add index file size
	if stat, err := os.Stat(indexPath); err == nil {
		files[0].SizeBytes = uint64(stat.Size())
	}

	for idx, t := range h.tables {
		numRows, numRowGroups, err := t.Flush()
		if err != nil {
			return errors.Wrapf(err, "flushing of table %s", t.Name())
		}
		h.metrics.rowsWritten.WithLabelValues(t.Name()).Add(float64(numRows))
		files[idx+1].Parquet = &block.ParquetFile{
			NumRowGroups: numRowGroups,
			NumRows:      numRows,
		}
	}

	for idx, t := range h.tables {
		if err := t.Close(); err != nil {
			return errors.Wrapf(err, "closing of table %s", t.Name())
		}

		// add file size
		files[idx+1].RelPath = t.Name() + block.ParquetSuffix
		if stat, err := os.Stat(filepath.Join(h.headPath, files[idx+1].RelPath)); err == nil {
			files[idx+1].SizeBytes = uint64(stat.Size())
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})
	h.meta.Files = files
	h.meta.Stats.NumProfiles = uint64(h.index.totalProfiles.Load())
	h.meta.Stats.NumSamples = h.totalSamples.Load()

	if _, err := h.meta.WriteToFile(h.logger, h.headPath); err != nil {
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
