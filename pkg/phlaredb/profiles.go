package phlaredb

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"unsafe"

	"github.com/gogo/status"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/samber/lo"
	"go.uber.org/atomic"
	"google.golang.org/grpc/codes"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	"github.com/grafana/phlare/pkg/iter"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/phlaredb/query"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
	"github.com/grafana/phlare/pkg/phlaredb/tsdb"
	"github.com/grafana/phlare/pkg/phlaredb/tsdb/index"
)

// delta encoding for ranges
type rowRange struct {
	rowNum int64
	length int
}

type rowRangeWithSeriesIndex struct {
	*rowRange
	seriesIndex uint32
}

// those need to be strictly ordered
type rowRangesWithSeriesIndex []rowRangeWithSeriesIndex

func (s rowRangesWithSeriesIndex) getSeriesIndex(rowNum int64) uint32 {
	l, r := 0, len(s)-1
	for l <= r {
		mid := (l + r) / 2
		if s[mid].rowRange == nil {
			l = mid + 1
			continue
		}
		if s[mid].rowNum <= rowNum && s[mid].rowNum+int64(s[mid].length) > rowNum {
			return s[mid].seriesIndex
		}
		if s[mid].rowNum > rowNum {
			r = mid - 1
		} else {
			l = mid + 1
		}
	}
	panic("series index not found")
}

type rowRanges map[rowRange]model.Fingerprint

func (rR rowRanges) iter() iter.Iterator[fingerprintWithRowNum] {
	// ensure row ranges is sorted
	rRSlice := lo.Keys(rR)
	sort.Slice(rRSlice, func(i, j int) bool {
		return rRSlice[i].rowNum < rRSlice[j].rowNum
	})

	fps := make([]model.Fingerprint, 0, len(rR))
	for _, elem := range rRSlice {
		fps = append(fps, rR[elem])
	}

	return &rowRangesIter{
		r:   rRSlice,
		fps: fps,
		pos: 0,
	}
}

type fingerprintWithRowNum struct {
	fp     model.Fingerprint
	rowNum int64
}

func (f fingerprintWithRowNum) RowNumber() int64 {
	return f.rowNum
}

func (r rowRanges) fingerprintsWithRowNum() query.Iterator {
	return query.NewRowNumberIterator(r.iter())
}

type rowRangesIter struct {
	r   []rowRange
	fps []model.Fingerprint
	pos int64
}

func (i *rowRangesIter) At() fingerprintWithRowNum {
	return fingerprintWithRowNum{
		rowNum: i.pos - 1,
		fp:     i.fps[0],
	}
}

func (i *rowRangesIter) Next() bool {
	if len(i.r) == 0 {
		return false
	}
	if i.pos < i.r[0].rowNum {
		i.pos = i.r[0].rowNum
	}

	if i.pos >= i.r[0].rowNum+int64(i.r[0].length) {
		i.r = i.r[1:]
		i.fps = i.fps[1:]
		return i.Next()
	}
	i.pos++
	return true
}

func (i *rowRangesIter) Close() error { return nil }

func (i *rowRangesIter) Err() error { return nil }

type profileSeries struct {
	lbs phlaremodel.Labels
	fp  model.Fingerprint

	minTime, maxTime int64

	// profiles in memory
	profiles []*schemav1.Profile

	// profiles temporary stored on disk in row group segements
	// TODO: this information is crucial to recover segements to a full block later
	profilesOnDisk []*rowRange
}

type profilesIndex struct {
	ix *tsdb.BitPrefixInvertedIndex
	// todo: like the inverted index we might want to shard fingerprint to avoid contentions.
	profilesPerFP   map[model.Fingerprint]*profileSeries
	mutex           sync.RWMutex
	totalProfiles   *atomic.Int64
	totalSeries     *atomic.Int64
	rowGroupsOnDisk int

	metrics *headMetrics
}

func newProfileIndex(totalShards uint32, metrics *headMetrics) (*profilesIndex, error) {
	ix, err := tsdb.NewBitPrefixWithShards(totalShards)
	if err != nil {
		return nil, err
	}
	return &profilesIndex{
		ix:            ix,
		profilesPerFP: make(map[model.Fingerprint]*profileSeries),
		totalProfiles: atomic.NewInt64(0),
		totalSeries:   atomic.NewInt64(0),
		metrics:       metrics,
	}, nil
}

// Add a new set of profile to the index.
// The seriesRef are expected to match the profile labels passed in.
func (pi *profilesIndex) Add(ps *schemav1.Profile, lbs phlaremodel.Labels, profileName string) {
	pi.mutex.Lock()
	defer pi.mutex.Unlock()
	profiles, ok := pi.profilesPerFP[ps.SeriesFingerprint]
	if !ok {
		lbs := pi.ix.Add(lbs, ps.SeriesFingerprint)
		profiles = &profileSeries{
			lbs:            lbs,
			fp:             ps.SeriesFingerprint,
			minTime:        ps.TimeNanos,
			maxTime:        ps.TimeNanos,
			profilesOnDisk: make([]*rowRange, pi.rowGroupsOnDisk),
		}
		pi.profilesPerFP[ps.SeriesFingerprint] = profiles
		pi.metrics.series.Set(float64(pi.totalSeries.Inc()))
		pi.metrics.seriesCreated.WithLabelValues(profileName).Inc()
	}

	profiles.profiles = append(profiles.profiles, ps)
	if ps.TimeNanos < profiles.minTime {
		profiles.minTime = ps.TimeNanos
	}
	if ps.TimeNanos > profiles.maxTime {
		profiles.maxTime = ps.TimeNanos
	}

	pi.metrics.profiles.Set(float64(pi.totalProfiles.Inc()))
	pi.metrics.profilesCreated.WithLabelValues(profileName).Inc()
}

func (pi *profilesIndex) selectMatchingFPs(ctx context.Context, params *ingestv1.SelectProfilesRequest) ([]model.Fingerprint, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "selectMatchingFPs - Index")
	defer sp.Finish()
	selectors, err := parser.ParseMetricSelector(params.LabelSelector)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to parse label selectors: "+err.Error())
	}
	selectors = append(selectors, phlaremodel.SelectorFromProfileType(params.Type))

	filters, matchers := SplitFiltersAndMatchers(selectors)
	ids, err := pi.ix.Lookup(matchers, nil)
	if err != nil {
		return nil, err
	}

	pi.mutex.RLock()
	defer pi.mutex.RUnlock()

	// filter fingerprints that no longer exist or don't match the filters
	var idx int
outer:
	for _, fp := range ids {
		profile, ok := pi.profilesPerFP[fp]
		if !ok {
			// If a profile labels is missing here, it has already been flushed
			// and is supposed to be picked up from storage by querier
			continue
		}
		for _, filter := range filters {
			if !filter.Matches(profile.lbs.Get(filter.Name)) {
				continue outer
			}
		}

		// keep this one
		ids[idx] = fp
		idx++
	}

	sp.SetTag("matchedSeries", idx)

	return ids[:idx], nil
}

func (pi *profilesIndex) selectMatchingRowRanges(ctx context.Context, params *ingestv1.SelectProfilesRequest, rowGroupIdx int) (
	query.Iterator,
	map[model.Fingerprint]phlaremodel.Labels,
	error,
) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "selectMatchingRowRanges - Index")
	defer sp.Finish()

	ids, err := pi.selectMatchingFPs(ctx, params)
	if err != nil {
		return nil, nil, err
	}

	// gather rowRanges and labels from matching series under read lock of the index
	var (
		rowRanges   = make(rowRanges, len(ids))
		labelsPerFP = make(map[model.Fingerprint]phlaremodel.Labels, len(ids))
	)

	pi.mutex.RLock()
	defer pi.mutex.RUnlock()

	for _, fp := range ids {
		// skip if series no longer in index
		profileSeries, ok := pi.profilesPerFP[fp]
		if !ok {
			continue
		}

		labelsPerFP[fp] = profileSeries.lbs

		// skip if rowRange empty
		rR := profileSeries.profilesOnDisk[rowGroupIdx]
		if rR == nil {
			continue
		}

		rowRanges[*rR] = fp
	}

	sp.SetTag("rowGroupSegment", rowGroupIdx)
	sp.SetTag("matchedRowRangesCount", len(rowRanges))

	return rowRanges.fingerprintsWithRowNum(), labelsPerFP, nil
}

type ProfileWithLabels struct {
	*schemav1.Profile
	lbs phlaremodel.Labels
	fp  model.Fingerprint
}

func (p ProfileWithLabels) Timestamp() model.Time {
	return model.TimeFromUnixNano(p.Profile.TimeNanos)
}

func (p ProfileWithLabels) Fingerprint() model.Fingerprint {
	return p.fp
}

func (p ProfileWithLabels) Labels() phlaremodel.Labels {
	return p.lbs
}

func (p ProfileWithLabels) Samples() []*schemav1.Sample {
	return p.Profile.Samples
}

type SeriesIterator struct {
	iter.Iterator[*schemav1.Profile]
	curr ProfileWithLabels
	fp   model.Fingerprint
	lbs  phlaremodel.Labels
}

func NewSeriesIterator(labels phlaremodel.Labels, fingerprint model.Fingerprint, it iter.Iterator[*schemav1.Profile]) *SeriesIterator {
	return &SeriesIterator{
		Iterator: it,
		fp:       fingerprint,
		lbs:      labels,
	}
}

func (it *SeriesIterator) Next() bool {
	if !it.Iterator.Next() {
		return false
	}
	it.curr = ProfileWithLabels{
		Profile: it.Iterator.At(),
		lbs:     it.lbs,
		fp:      it.fp,
	}
	return true
}

func (it *SeriesIterator) At() Profile {
	return it.curr
}

// forMatchingLabels iterates through all matching label sets and calls f for each labels set.
func (pi *profilesIndex) forMatchingLabels(matchers []*labels.Matcher,
	fn func(lbs phlaremodel.Labels, fp model.Fingerprint) error,
) error {
	filters, matchers := SplitFiltersAndMatchers(matchers)
	ids, err := pi.ix.Lookup(matchers, nil)
	if err != nil {
		return err
	}

	pi.mutex.RLock()
	defer pi.mutex.RUnlock()

outer:
	for _, fp := range ids {
		profile, ok := pi.profilesPerFP[fp]
		if !ok {
			// If a profile labels is missing here, it has already been flushed
			// and is supposed to be picked up from storage by querier
			continue
		}
		for _, filter := range filters {
			if !filter.Matches(profile.lbs.Get(filter.Name)) {
				continue outer
			}
		}
		if err := fn(profile.lbs, fp); err != nil {
			return err
		}
	}
	return nil
}

// WriteTo writes the profiles tsdb index to the specified filepath.
func (pi *profilesIndex) writeTo(ctx context.Context, path string) ([][]rowRangeWithSeriesIndex, error) {
	writer, err := index.NewWriter(ctx, path)
	if err != nil {
		return nil, err
	}
	pi.mutex.RLock()
	defer pi.mutex.RUnlock()

	pfs := make([]*profileSeries, 0, len(pi.profilesPerFP))

	for _, p := range pi.profilesPerFP {
		pfs = append(pfs, p)
	}

	// sort by fp
	sort.Slice(pfs, func(i, j int) bool {
		return phlaremodel.CompareLabelPairs(pfs[i].lbs, pfs[j].lbs) < 0
	})

	symbolsMap := make(map[string]struct{})
	for _, s := range pfs {
		for _, l := range s.lbs {
			symbolsMap[l.Name] = struct{}{}
			symbolsMap[l.Value] = struct{}{}
		}
	}

	// Sort symbols
	symbols := make([]string, 0, len(symbolsMap))
	for s := range symbolsMap {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	// Add symbols
	for _, symbol := range symbols {
		if err := writer.AddSymbol(symbol); err != nil {
			return nil, err
		}
	}

	// ranges per row group
	rangesPerRG := make([][]rowRangeWithSeriesIndex, len(pfs[0].profilesOnDisk))

	// Add series
	for i, s := range pfs {
		if err := writer.AddSeries(storage.SeriesRef(i), s.lbs, s.fp, index.ChunkMeta{
			MinTime: s.minTime,
			MaxTime: s.maxTime,
			// We store the series Index from the head with the series to use when retrieving data from parquet.
			SeriesIndex: uint32(i),
		}); err != nil {
			return nil, err
		}
		// store series index
		for idx, rg := range s.profilesOnDisk {
			rangesPerRG[idx] = append(rangesPerRG[idx], rowRangeWithSeriesIndex{rowRange: rg, seriesIndex: uint32(i)})
		}
	}

	return rangesPerRG, writer.Close()
}

func (pl *profilesIndex) cutRowGroup(rgProfiles []*schemav1.Profile) error {
	// adding rowGroup and rowNum information per fingerprint
	rowRangePerFP := make(map[model.Fingerprint]*rowRange, len(pl.profilesPerFP))
	for rowNum, p := range rgProfiles {
		if _, ok := rowRangePerFP[p.SeriesFingerprint]; !ok {
			rowRangePerFP[p.SeriesFingerprint] = &rowRange{
				rowNum: int64(rowNum),
			}
		}

		rowRange := rowRangePerFP[p.SeriesFingerprint]
		rowRange.length++

		// sanity check
		if (int(rowRange.rowNum) + rowRange.length - 1) != rowNum {
			return fmt.Errorf("rowRange is not matching up, ensure that the ordering of the profile row group is ordered correctly, current row_num=%d, expect range %d-%d", rowNum, rowRange.rowNum, int(rowRange.rowNum)+rowRange.length)
		}
	}

	pl.mutex.Lock()
	defer pl.mutex.Unlock()

	pl.rowGroupsOnDisk += 1

	for _, ps := range pl.profilesPerFP {
		// empty all in memory profiles
		ps.profiles = ps.profiles[:0]

		// attach rowGroup and rowNum information
		rowRange := rowRangePerFP[ps.fp]

		ps.profilesOnDisk = append(
			ps.profilesOnDisk,
			rowRange,
		)
	}

	return nil
}

// SplitFiltersAndMatchers splits empty matchers off, which are treated as filters, see #220
func SplitFiltersAndMatchers(allMatchers []*labels.Matcher) (filters, matchers []*labels.Matcher) {
	for _, matcher := range allMatchers {
		// If a matcher matches "", we need to fetch possible chunks where
		// there is no value and will therefore not be in our label index.
		// e.g. {foo=""} and {foo!="bar"} both match "", so we need to return
		// chunks which do not have a foo label set. When looking entries in
		// the index, we should ignore this matcher to fetch all possible chunks
		// and then filter on the matcher after the chunks have been fetched.
		if matcher.Matches("") {
			filters = append(filters, matcher)
		} else {
			matchers = append(matchers, matcher)
		}
	}
	return
}

// nolint unused
const (
	profileSize = uint64(unsafe.Sizeof(schemav1.Profile{}))
	sampleSize  = uint64(unsafe.Sizeof(schemav1.Sample{}))
)

type profilesHelper struct{}

// nolint unused
func (*profilesHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.locations = elemRewriter
}

// nolint unused
func (*profilesHelper) rewrite(r *rewriter, s *schemav1.Profile) error {
	for pos := range s.Comments {
		r.strings.rewrite(&s.Comments[pos])
	}

	r.strings.rewrite(&s.DropFrames)
	r.strings.rewrite(&s.KeepFrames)

	return nil
}

// nolint unused
func (*profilesHelper) setID(oldID, newID uint64, p *schemav1.Profile) uint64 {
	return oldID
}

// nolint unused
func sizeOfSample(s *schemav1.Sample) uint64 {
	return sampleSize + 8
}

// nolint unused
func (*profilesHelper) size(p *schemav1.Profile) uint64 {
	size := profileSize

	size += 8
	size += uint64(len(p.Comments) * 8)

	for _, s := range p.Samples {
		size += sizeOfSample(s)
	}

	return size
}

// nolint unused
func (*profilesHelper) clone(p *schemav1.Profile) *schemav1.Profile {
	return p
}
