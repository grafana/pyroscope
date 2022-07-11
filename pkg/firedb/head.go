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
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/segmentio/parquet-go"
	"google.golang.org/grpc/codes"

	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	firemodel "github.com/grafana/fire/pkg/model"
)

type idConversionTable map[int64]int64

func (t stringConversionTable) rewritePprofLabels(in []*profilev1.Label) []*profilev1.Label {
	out := make([]*profilev1.Label, len(in))
	for pos := range in {
		out[pos] = &profilev1.Label{
			Key:     t[in[pos].Key],
			NumUnit: t[in[pos].NumUnit],
			Str:     t[in[pos].Str],
			Num:     in[pos].Num,
		}
	}

	return out
}

func (t idConversionTable) rewrite(idx *int64) {
	pos := *idx
	*idx = t[pos]
}

func (t idConversionTable) rewriteUint64(idx *uint64) {
	pos := *idx
	*idx = uint64(t[int64(pos)])
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
}

type deduplicatingSlice[M Models, K comparable, H Helper[M, K]] struct {
	slice  []M
	lock   sync.RWMutex
	lookup map[K]int64
}

func (s *deduplicatingSlice[M, K, H]) init() {
	s.lookup = make(map[K]int64)
}

func (s *deduplicatingSlice[M, K, H]) ingest(ctx context.Context, elems []M, rewriter *rewriter) error {
	var (
		missing      []int64
		rewritingMap = make(map[int64]int64)
		h            H
	)

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
			rewritingMap[int64(pos)] = posSlice
		} else {
			missing = append(missing, int64(pos))
		}
	}
	s.lock.RUnlock()

	// if there are missing elements, acquire write lock
	if len(missing) > 0 {
		s.lock.Lock()
		posSlice := int64(len(s.slice))
		for _, pos := range missing {
			// check again if element exists
			k := h.key(elems[pos])
			if posSlice, exists := s.lookup[k]; exists {
				rewritingMap[pos] = posSlice
				continue
			}

			// add element to slice/map
			s.slice = append(s.slice, elems[pos])
			s.lookup[k] = posSlice
			rewritingMap[pos] = posSlice
			posSlice++
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
	logger log.Logger

	index       *profilesIndex
	strings     deduplicatingSlice[string, string, *stringsHelper]
	mappings    deduplicatingSlice[*profilev1.Mapping, mappingsKey, *mappingsHelper]
	functions   deduplicatingSlice[*profilev1.Function, functionsKey, *functionsHelper]
	locations   deduplicatingSlice[*profilev1.Location, locationsKey, *locationsHelper]
	stacktraces deduplicatingSlice[*schemav1.Stacktrace, stacktracesKey, *stacktracesHelper] // a stacktrace is a slice of location ids
	profiles    deduplicatingSlice[*schemav1.Profile, profilesKey, *profilesHelper]
}

func NewHead() (*Head, error) {
	index, err := newProfileIndex(32)
	if err != nil {
		return nil, err
	}

	h := &Head{
		logger: log.NewLogfmtLogger(os.Stderr),
		index:  index,
	}
	h.strings.init()
	h.mappings.init()
	h.functions.init()
	h.locations.init()
	h.stacktraces.init()
	h.profiles.init()
	return h, nil
}

type LabelPairRef struct {
	Name  int64
	Value int64
}

// resolves external labels into string slice
func (h *Head) internExternalLabels(ctx context.Context, lblStrs []*commonv1.LabelPair) ([]LabelPairRef, error) {
	var (
		strs    = make([]string, len(lblStrs)*2)
		lblRefs = make([]LabelPairRef, len(lblStrs))
	)

	for pos := range lblStrs {
		strs[(pos * 2)] = lblStrs[pos].Name
		strs[(pos*2)+1] = lblStrs[pos].Value
	}

	// ensure labels are in string table
	r := &rewriter{}
	if err := h.strings.ingest(ctx, strs, r); err != nil {
		return nil, err
	}

	for pos := range lblRefs {
		lblRefs[pos].Name = r.strings[(pos * 2)]
		lblRefs[pos].Value = r.strings[(pos*2)+1]
	}

	return lblRefs, nil
}

func (h *Head) convertSamples(ctx context.Context, r *rewriter, in []*profilev1.Sample) ([]*schemav1.Sample, error) {
	var (
		out         = make([]*schemav1.Sample, len(in))
		stacktraces = make([]*schemav1.Stacktrace, len(in))
	)

	for pos := range in {
		// populate samples
		out[pos] = &schemav1.Sample{
			Values: in[pos].Value,
			Labels: r.strings.rewritePprofLabels(in[pos].Label),
		}

		// build full stack traces
		stacktraces[pos] = &schemav1.Stacktrace{
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
		Comment:           p.Comment,
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
			fnIds := make([]int32, len(locs))
			for i, loc := range locs {
				fnName := h.strings.slice[h.functions.slice[h.locations.slice[loc].Line[0].FunctionId].Name]
				pos, ok := namesPositions[fnName]
				if !ok {
					namesPositions[fnName] = len(names)
					fnIds[i] = int32(len(names))
					names = append(names, fnName)
					continue
				}
				fnIds[i] = int32(pos)
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

	file, err := os.OpenFile(filepath.Join(path, "profiles"+".parquet"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	wProfiles := schemav1.Writer[*schemav1.Profile, *schemav1.ProfilePersister]{}
	if err := wProfiles.WriteParquetFile(file, h.profiles.slice); err != nil {
		return err
	}

	file, err = os.OpenFile(filepath.Join(path, "stacktraces"+".parquet"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	wStacktraces := schemav1.Writer[*schemav1.Stacktrace, *schemav1.StacktracePersister]{}
	if err := wStacktraces.WriteParquetFile(file, h.stacktraces.slice); err != nil {
		return err
	}

	strings := schemav1.Strings(h.strings.slice)
	if err := writeToFile(ctx, path, "strings",
		[]parquet.RowGroupOption{
			schemav1.StringsSchema(),
			schemav1.StringsSorting(),
		},
		[]parquet.WriterOption{
			schemav1.StringsSchema(),
		},
		strings.ToRows(),
	); err != nil {
		return err
	}

	if err := writeToFile(ctx, path, "mappings", nil, nil, h.mappings.slice); err != nil {
		return err
	}

	if err := writeToFile(ctx, path, "locations", nil, nil, h.locations.slice); err != nil {
		return err
	}

	if err := writeToFile(ctx, path, "functions", nil, nil, h.functions.slice); err != nil {
		return err
	}

	return nil
}

func writeToFile[T any](ctx context.Context, path string, table string, rowGroupOptions []parquet.RowGroupOption, writerOptions []parquet.WriterOption, rows []T) error {
	file, err := os.OpenFile(filepath.Join(path, table+".parquet"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	buffer := parquet.NewGenericBuffer[T](rowGroupOptions...)
	if _, err := buffer.Write(rows); err != nil {
		return err
	}
	sort.Sort(buffer)

	writer := parquet.NewGenericWriter[T](file, writerOptions...)
	if _, err := parquet.CopyRows(writer, buffer.Rows()); err != nil {
		return err
	}

	return writer.Close()
}
