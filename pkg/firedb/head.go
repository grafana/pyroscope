package firedb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/segmentio/parquet-go"

	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
)

type idConversionTable map[int64]int64

func (t stringConversionTable) rewritePprofLabels(in []*profilev1.Label) []*profilev1.Label {
	var (
		out = make([]*profilev1.Label, len(in))
	)
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
		var posSlice = int64(len(s.slice))
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

	strings     deduplicatingSlice[string, string, *stringsHelper]
	mappings    deduplicatingSlice[*profilev1.Mapping, mappingsKey, *mappingsHelper]
	functions   deduplicatingSlice[*profilev1.Function, functionsKey, *functionsHelper]
	locations   deduplicatingSlice[*profilev1.Location, locationsKey, *locationsHelper]
	stacktraces deduplicatingSlice[*Stacktrace, stacktracesKey, *stacktracesHelper] // a stacktrace is a slice of location ids
	profiles    deduplicatingSlice[*Profile, profilesKey, *profilesHelper]
}

func NewHead() *Head {
	h := &Head{
		logger: log.NewLogfmtLogger(os.Stderr),
	}
	h.strings.init()
	h.mappings.init()
	h.functions.init()
	h.locations.init()
	h.stacktraces.init()
	h.profiles.init()
	return h
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

	lblRefs, err := h.internExternalLabels(ctx, externalLabels)
	if err != nil {
		return err
	}

	samples, err := h.convertSamples(ctx, rewrites, p.Sample)
	if err != nil {
		return err
	}

	profile := &schemav1.Profile{
		SeriesRefs:        seriesRefs,
		Samples:           samples,
		Types:             p.SampleType,
		DropFrames:        p.DropFrames,
		KeepFrames:        p.KeepFrames,
		TimeNanos:         p.TimeNanos,
		DurationNanos:     p.DurationNanos,
		PeriodType:        p.PeriodType,
		Period:            p.Period,
		Comment:           p.Comment,
		DefaultSampleType: p.DefaultSampleType,
	}

	if err := h.profiles.ingest(ctx, []*schemav1.Profile{profile}, rewrites); err != nil {
		return err
	}

	return nil
}

// LabelValues returns the possible label values for a given label name.
func (h *Head) LabelValues(ctx context.Context, req *connect.Request[ingestv1.LabelValuesRequest]) (*connect.Response[ingestv1.LabelValuesResponse], error) {

	idx, ok := h.strings.getIndex(req.Msg.Name)
	if !ok {
		return connect.NewResponse(&ingestv1.LabelValuesResponse{Names: []string{}}), nil
	}

	valsMap := make(map[int64]struct{})
	for _, profile := range h.profiles.slice {
		for _, lbl := range profile.ExternalLabels {
			if lbl.Name == idx {
				valsMap[lbl.Value] = struct{}{}
			}
		}
	}

	vals := make([]string, len(valsMap))
	idx = 0
	for v := range valsMap {
		vals[idx] = h.strings.slice[v]
		idx++
	}
	sort.Strings(vals)
	return connect.NewResponse(&ingestv1.LabelValuesResponse{
		Names: vals,
	}), nil
}

type profileTypeSeen struct {
	Name       int64
	SampleType int64
	SampleUnit int64
	PeriodType int64
	PeriodUnit int64
}

func (pt *profileTypeSeen) String(t []string) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s",
		t[pt.Name],
		t[pt.SampleType],
		t[pt.SampleUnit],
		t[pt.PeriodType],
		t[pt.PeriodUnit],
	)
}

// ProfileTypes returns the possible profile types.
func (h *Head) ProfileTypes(ctx context.Context, req *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {

	idxLabelName, ok := h.strings.getIndex("__name__")
	if !ok {
		return nil, errors.New("No name label found")
	}

	seen := map[profileTypeSeen]struct{}{}

	var nameRef int64
	for _, profile := range h.profiles.slice {
		nameRef = 0
		for _, lbl := range profile.ExternalLabels {
			if lbl.Name == idxLabelName {
				nameRef = lbl.Value
			}
		}

		for _, typ := range profile.Types {
			seen[profileTypeSeen{
				Name:       nameRef,
				SampleType: typ.Type,
				SampleUnit: typ.Unit,
				PeriodType: profile.PeriodType.Type,
				PeriodUnit: profile.PeriodType.Unit,
			}] = struct{}{}
		}
	}

	types := make([]string, 0, len(seen))
	for key := range seen {
		types = append(types, key.String(h.strings.slice))
	}
	sort.Strings(types)
	return connect.NewResponse(&ingestv1.ProfileTypesResponse{
		Names: types,
	}), nil

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

	if err := writeToFile(ctx, path, "samples",
		[]parquet.RowGroupOption{
			schemav1.ProfilesSchema(),
			parquet.SortingColumns(
				parquet.Ascending("ID"),
				parquet.Ascending("TimeNanos"),
			)},
		[]parquet.WriterOption{
			schemav1.ProfilesSchema(),
		},
		h.profiles.slice,
	); err != nil {
		return err
	}

	if err := writeToFile(ctx, path, "strings", nil, nil, stringSliceToRows(h.strings.slice)); err != nil {
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
	file, err := os.OpenFile(filepath.Join(path, table+".parquet"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
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
