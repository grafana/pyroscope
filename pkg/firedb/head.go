package firedb

import (
	"context"
	"fmt"
	"io"
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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"go.uber.org/atomic"
	"google.golang.org/grpc/codes"

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

type deduplicatingSlice[M Models, K comparable, H Helper[M, K]] struct {
	slice  []M
	size   atomic.Uint64
	lock   sync.RWMutex
	lookup map[K]int64

	missing []int64
}

func (s *deduplicatingSlice[M, K, H]) init() {
	s.lookup = make(map[K]int64)
}

func (s *deduplicatingSlice[M, K, H]) ingest(ctx context.Context, elems []M, rewriter *rewriter) error {
	var (
		rewritingMap = make(map[int64]int64)
		h            H
	)
	if s.missing == nil {
		s.missing = make([]int64, 0, len(elems))
	}
	s.missing = s.missing[:0]

	// rewrite elements
	for pos := range elems {
		if err := h.rewrite(rewriter, elems[pos]); err != nil {
			return err
		}
	}

	// try to find if element already exists in slice
	s.lock.RLock()
	for pos := range elems {
		k := h.key(elems[pos])
		if posSlice, exists := s.lookup[k]; exists {
			rewritingMap[int64(h.setID(uint64(pos), uint64(posSlice), elems[pos]))] = posSlice
		} else {
			s.missing = append(s.missing, int64(pos))
		}
	}
	s.lock.RUnlock()

	// if there are missing elements, acquire write lock
	if len(s.missing) > 0 {
		s.lock.Lock()
		posSlice := int64(len(s.slice))
		for _, pos := range s.missing {
			// check again if element exists
			k := h.key(elems[pos])
			if posSlice, exists := s.lookup[k]; exists {
				rewritingMap[int64(h.setID(uint64(pos), uint64(posSlice), elems[pos]))] = posSlice
				continue
			}

			// add element to slice/map
			s.slice = append(s.slice, h.clone(elems[pos]))
			s.lookup[k] = posSlice
			rewritingMap[int64(h.setID(uint64(pos), uint64(posSlice), elems[pos]))] = posSlice
			posSlice++

			// increase size of stored data
			s.size.Add(h.size(elems[pos]))
		}
		s.lock.Unlock()
	}

	// add rewrite information to struct
	h.addToRewriter(rewriter, rewritingMap)

	return nil
}

func (s *deduplicatingSlice[M, K, H]) getIndex(key K) (int64, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	v, ok := s.lookup[key]
	return v, ok
}

type Head struct {
	logger  log.Logger
	metrics *headMetrics

	index           *profilesIndex
	strings         deduplicatingSlice[string, string, *stringsHelper]
	mappings        deduplicatingSlice[*profilev1.Mapping, mappingsKey, *mappingsHelper]
	functions       deduplicatingSlice[*profilev1.Function, functionsKey, *functionsHelper]
	locations       deduplicatingSlice[*profilev1.Location, locationsKey, *locationsHelper]
	stacktraces     deduplicatingSlice[*schemav1.Stacktrace, stacktracesKey, *stacktracesHelper] // a stacktrace is a slice of location ids
	profiles        deduplicatingSlice[*schemav1.Profile, profilesKey, *profilesHelper]
	pprofLabelCache labelCache
}

func NewHead(reg prometheus.Registerer) (*Head, error) {
	h := &Head{
		logger: log.NewLogfmtLogger(os.Stderr),
	}
	h.strings.init()
	h.mappings.init()
	h.functions.init()
	h.locations.init()
	h.stacktraces.init()
	h.profiles.init()
	h.metrics = newHeadMetrics(h, reg)
	index, err := newProfileIndex(32, h.metrics)
	if err != nil {
		return nil, err
	}
	h.index = index

	h.pprofLabelCache.init()
	return h, nil
}

func (h *Head) convertSamples(ctx context.Context, r *rewriter, in []*profilev1.Sample) ([]*schemav1.Sample, error) {
	var (
		out         = make([]*schemav1.Sample, len(in))
		stacktraces = make([]*schemav1.Stacktrace, len(in))
	)

	for pos := range in {
		// populate samples
		out[pos] = &schemav1.Sample{
			Values: copySlice(in[pos].Value),
			Labels: h.pprofLabelCache.rewriteLabels(r.strings, in[pos].Label),
		}

		// build full stack traces
		stacktraces[pos] = &schemav1.Stacktrace{
			// no copySlice necessary at this point,stacktracesHelper.clone
			// will copy it, if it is required to be retained.
			LocationIDs: in[pos].LocationId,
		}
	}

	// ingest stacktraces
	if err := h.stacktraces.ingest(ctx, stacktraces, r); err != nil {
		return nil, err
	}

	// reference stacktraces
	for pos := range out {
		out[pos].StacktraceID = uint64(r.stacktraces[int64(pos)])
	}

	return out, nil
}

func (h *Head) Ingest(ctx context.Context, p *profilev1.Profile, externalLabels ...*commonv1.LabelPair) error {
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

	seriesRefs := make([]model.Fingerprint, len(p.SampleType))
	profilesLabels := make([]firemodel.Labels, len(p.SampleType))
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

	samples, err := h.convertSamples(ctx, rewrites, p.Sample)
	if err != nil {
		return err
	}

	profile := &schemav1.Profile{
		ID:                uuid.New(),
		SeriesRefs:        seriesRefs,
		Samples:           samples,
		DropFrames:        p.DropFrames,
		KeepFrames:        p.KeepFrames,
		TimeNanos:         p.TimeNanos,
		DurationNanos:     p.DurationNanos,
		Comment:           copySlice(p.Comment),
		DefaultSampleType: p.DefaultSampleType,
	}

	if err := h.profiles.ingest(ctx, []*schemav1.Profile{profile}, rewrites); err != nil {
		return err
	}
	h.index.Add(profile, profilesLabels)

	return nil
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

// ProfileTypes returns the possible profile types.
func (h *Head) ProfileTypes(ctx context.Context, req *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
	values, err := h.index.ix.LabelValues(firemodel.LabelNameProfileType, nil)
	if err != nil {
		return nil, err
	}
	sort.Strings(values)

	return connect.NewResponse(&ingestv1.ProfileTypesResponse{
		Names: values,
	}), nil
}

func (h *Head) SelectProfiles(ctx context.Context, req *connect.Request[ingestv1.SelectProfilesRequest]) (*connect.Response[ingestv1.SelectProfilesResponse], error) {
	selectors, err := parser.ParseMetricSelector(req.Msg.LabelSelector)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to label selector")
	}
	selectors = append(selectors, &labels.Matcher{
		Type:  labels.MatchEqual,
		Name:  firemodel.LabelNameProfileType,
		Value: req.Msg.Type.Name + ":" + req.Msg.Type.SampleType + ":" + req.Msg.Type.SampleUnit + ":" + req.Msg.Type.PeriodType + ":" + req.Msg.Type.PeriodUnit,
	})

	result := []*ingestv1.Profile{}
	names := []string{}
	namesPositions := map[string]int{}

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

	err = h.index.forMatchingProfiles(selectors, func(lbs firemodel.Labels, _ model.Fingerprint, idx int, profile *schemav1.Profile) error {
		ts := int64(model.TimeFromUnixNano(profile.TimeNanos))
		// if the timestamp is not matching we skip this profile.
		if req.Msg.Start > ts || ts > req.Msg.End {
			return nil
		}
		p := &ingestv1.Profile{
			Type:        req.Msg.Type,
			Labels:      lbs,
			Timestamp:   ts,
			Stacktraces: make([]*ingestv1.StacktraceSample, 0, len(profile.Samples)),
		}
		for _, s := range profile.Samples {
			locs := h.stacktraces.slice[s.StacktraceID].LocationIDs
			fnIds := make([]int32, 0, len(locs))
			for _, loc := range locs {
				for _, line := range h.locations.slice[loc].Line {
					fnName := h.strings.slice[h.functions.slice[line.FunctionId].Name]
					pos, ok := namesPositions[fnName]
					if !ok {
						namesPositions[fnName] = len(names)
						fnIds = append(fnIds, int32(len(names)))
						names = append(names, fnName)
						continue
					}
					fnIds = append(fnIds, int32(pos))
				}
			}
			p.Stacktraces = append(p.Stacktraces, &ingestv1.StacktraceSample{
				Value:       s.Values[idx],
				FunctionIds: fnIds,
			})
		}
		result = append(result, p)
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

func (h *Head) WriteTo(ctx context.Context, path string) error {
	level.Info(h.logger).Log("msg", "write head to disk", "path", path)

	fileInfo, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !fileInfo.IsDir() {
		return fmt.Errorf("error %s is no directory", path)
	}

	// loop over existing tables and write the parquet files sequentially
	for _, table := range []struct {
		name string
		f    func(w io.Writer) error
	}{
		{
			name: "profiles",
			f: func(f io.Writer) error {
				w := schemav1.ReadWriter[*schemav1.Profile, *schemav1.ProfilePersister]{}
				return w.WriteParquetFile(f, h.profiles.slice)
			},
		},
		{
			name: "stacktraces",
			f: func(f io.Writer) error {
				w := schemav1.ReadWriter[*schemav1.Stacktrace, *schemav1.StacktracePersister]{}
				return w.WriteParquetFile(f, h.stacktraces.slice)
			},
		},
		{
			name: "strings",
			f: func(f io.Writer) error {
				w := schemav1.ReadWriter[string, *schemav1.StringPersister]{}
				return w.WriteParquetFile(f, h.strings.slice)
			},
		},
		{
			name: "mappings",
			f: func(f io.Writer) error {
				w := schemav1.ReadWriter[*profilev1.Mapping, *schemav1.MappingPersister]{}
				return w.WriteParquetFile(f, h.mappings.slice)
			},
		},
		{
			name: "locations",
			f: func(f io.Writer) error {
				w := schemav1.ReadWriter[*profilev1.Location, *schemav1.LocationPersister]{}
				return w.WriteParquetFile(f, h.locations.slice)
			},
		},
		{
			name: "functions",
			f: func(f io.Writer) error {
				w := schemav1.ReadWriter[*profilev1.Function, *schemav1.FunctionPersister]{}
				return w.WriteParquetFile(f, h.functions.slice)
			},
		},
	} {
		file, err := os.OpenFile(filepath.Join(path, table.name+".parquet"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}

		if err := table.f(file); err != nil {
			return err
		}

		if err := file.Close(); err != nil {
			return err
		}
	}

	return nil
}
