package pprof

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/cespare/xxhash/v2"
	"github.com/colega/zeropool"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/pprof/profile"
	"github.com/klauspost/compress/gzip"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/slices"
	"github.com/grafana/pyroscope/pkg/util"
)

var (
	gzipReaderPool = sync.Pool{
		New: func() any {
			return &gzipReader{
				reader: bytes.NewReader(nil),
			}
		},
	}
	gzipWriterPool = sync.Pool{
		New: func() any {
			return gzip.NewWriter(io.Discard)
		},
	}
	bufPool = sync.Pool{
		New: func() any {
			return bytes.NewBuffer(nil)
		},
	}
)

type gzipReader struct {
	gzip   *gzip.Reader
	reader *bytes.Reader
}

// open gzip, create reader if required
func (r *gzipReader) gzipOpen() error {
	var err error
	if r.gzip == nil {
		r.gzip, err = gzip.NewReader(r.reader)
	} else {
		err = r.gzip.Reset(r.reader)
	}
	return err
}

func (r *gzipReader) openBytes(input []byte) (io.Reader, error) {
	r.reader.Reset(input)

	// handle if data is not gzipped at all
	if err := r.gzipOpen(); err == gzip.ErrHeader {
		r.reader.Reset(input)
		return r.reader, nil
	} else if err != nil {
		return nil, errors.Wrap(err, "gzip reset")
	}

	return r.gzip, nil
}

func NewProfile() *Profile {
	return RawFromProto(new(profilev1.Profile))
}

func RawFromProto(pbp *profilev1.Profile) *Profile {
	return &Profile{Profile: pbp}
}

func RawFromBytes(input []byte) (_ *Profile, err error) {
	gzipReader := gzipReaderPool.Get().(*gzipReader)
	buf := bufPool.Get().(*bytes.Buffer)
	defer func() {
		gzipReaderPool.Put(gzipReader)
		buf.Reset()
		bufPool.Put(buf)
	}()

	r, err := gzipReader.openBytes(input)
	if err != nil {
		return nil, err
	}

	if _, err = io.Copy(buf, r); err != nil {
		return nil, errors.Wrap(err, "copy to buffer")
	}

	rawSize := buf.Len()
	pbp := new(profilev1.Profile)
	if err = pbp.UnmarshalVT(buf.Bytes()); err != nil {
		return nil, err
	}

	return &Profile{
		Profile: pbp,
		rawSize: rawSize,
	}, nil
}

func FromBytes(input []byte, fn func(*profilev1.Profile, int) error) error {
	p, err := RawFromBytes(input)
	if err != nil {
		return err
	}
	return fn(p.Profile, p.rawSize)
}

func FromProfile(p *profile.Profile) (*profilev1.Profile, error) {
	var r profilev1.Profile
	strings := make(map[string]int)

	r.Sample = make([]*profilev1.Sample, 0, len(p.Sample))
	r.SampleType = make([]*profilev1.ValueType, 0, len(p.SampleType))
	r.Location = make([]*profilev1.Location, 0, len(p.Location))
	r.Mapping = make([]*profilev1.Mapping, 0, len(p.Mapping))
	r.Function = make([]*profilev1.Function, 0, len(p.Function))

	addString(strings, "")
	for _, st := range p.SampleType {
		r.SampleType = append(r.SampleType, &profilev1.ValueType{
			Type: addString(strings, st.Type),
			Unit: addString(strings, st.Unit),
		})
	}
	for _, s := range p.Sample {
		sample := &profilev1.Sample{
			LocationId: make([]uint64, len(s.Location)),
			Value:      s.Value,
		}
		for i, loc := range s.Location {
			sample.LocationId[i] = loc.ID
		}
		var keys []string
		for k := range s.Label {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			vs := s.Label[k]
			for _, v := range vs {
				sample.Label = append(sample.Label,
					&profilev1.Label{
						Key: addString(strings, k),
						Str: addString(strings, v),
					},
				)
			}
		}
		var numKeys []string
		for k := range s.NumLabel {
			numKeys = append(numKeys, k)
		}
		sort.Strings(numKeys)
		for _, k := range numKeys {
			keyX := addString(strings, k)
			vs := s.NumLabel[k]
			units := s.NumUnit[k]
			for i, v := range vs {
				var unitX int64
				if len(units) != 0 {
					unitX = addString(strings, units[i])
				}
				sample.Label = append(sample.Label,
					&profilev1.Label{
						Key:     keyX,
						Num:     v,
						NumUnit: unitX,
					},
				)
			}
		}
		r.Sample = append(r.Sample, sample)
	}

	for _, m := range p.Mapping {
		r.Mapping = append(r.Mapping, &profilev1.Mapping{
			Id:              m.ID,
			Filename:        addString(strings, m.File),
			MemoryStart:     m.Start,
			MemoryLimit:     m.Limit,
			FileOffset:      m.Offset,
			BuildId:         addString(strings, m.BuildID),
			HasFunctions:    m.HasFunctions,
			HasFilenames:    m.HasFilenames,
			HasLineNumbers:  m.HasLineNumbers,
			HasInlineFrames: m.HasInlineFrames,
		})
	}

	for _, l := range p.Location {
		loc := &profilev1.Location{
			Id:       l.ID,
			Line:     make([]*profilev1.Line, len(l.Line)),
			IsFolded: l.IsFolded,
			Address:  l.Address,
		}
		if l.Mapping != nil {
			loc.MappingId = l.Mapping.ID
		}
		for i, ln := range l.Line {
			if ln.Function != nil {
				loc.Line[i] = &profilev1.Line{
					FunctionId: ln.Function.ID,
					Line:       ln.Line,
				}
			} else {
				loc.Line[i] = &profilev1.Line{
					FunctionId: 0,
					Line:       ln.Line,
				}
			}
		}
		r.Location = append(r.Location, loc)
	}
	for _, f := range p.Function {
		r.Function = append(r.Function, &profilev1.Function{
			Id:         f.ID,
			Name:       addString(strings, f.Name),
			SystemName: addString(strings, f.SystemName),
			Filename:   addString(strings, f.Filename),
			StartLine:  f.StartLine,
		})
	}

	r.DropFrames = addString(strings, p.DropFrames)
	r.KeepFrames = addString(strings, p.KeepFrames)

	if pt := p.PeriodType; pt != nil {
		r.PeriodType = &profilev1.ValueType{
			Type: addString(strings, pt.Type),
			Unit: addString(strings, pt.Unit),
		}
	}

	for _, c := range p.Comments {
		r.Comment = append(r.Comment, addString(strings, c))
	}

	r.DefaultSampleType = addString(strings, p.DefaultSampleType)
	r.DurationNanos = p.DurationNanos
	r.TimeNanos = p.TimeNanos
	r.Period = p.Period
	r.StringTable = make([]string, len(strings))
	for s, i := range strings {
		r.StringTable[i] = s
	}
	return &r, nil
}

func addString(strings map[string]int, s string) int64 {
	i, ok := strings[s]
	if !ok {
		i = len(strings)
		strings[s] = i
	}
	return int64(i)
}

func OpenFile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return RawFromBytes(data)
}

type Profile struct {
	*profilev1.Profile
	hasher  SampleHasher
	rawSize int
}

// WriteTo writes the profile to the given writer.
func (p *Profile) WriteTo(w io.Writer) (int64, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		bufPool.Put(buf)
	}()
	buf.Grow(p.SizeVT())
	data := buf.Bytes()
	n, err := p.MarshalToVT(data)
	if err != nil {
		return 0, err
	}
	data = data[:n]

	gzipWriter := gzipWriterPool.Get().(*gzip.Writer)
	gzipWriter.Reset(w)
	defer func() {
		// reset gzip writer and return to pool
		gzipWriter.Reset(io.Discard)
		gzipWriterPool.Put(gzipWriter)
	}()

	written, err := gzipWriter.Write(data)
	if err != nil {
		return 0, errors.Wrap(err, "gzip write")
	}
	if err := gzipWriter.Close(); err != nil {
		return 0, errors.Wrap(err, "gzip close")
	}
	return int64(written), nil
}

type sortedSample struct {
	samples []*profilev1.Sample
	hashes  []uint64
}

func (s *sortedSample) Len() int {
	return len(s.samples)
}

func (s *sortedSample) Less(i, j int) bool {
	return s.hashes[i] < s.hashes[j]
}

func (s *sortedSample) Swap(i, j int) {
	s.samples[i], s.samples[j] = s.samples[j], s.samples[i]
	s.hashes[i], s.hashes[j] = s.hashes[j], s.hashes[i]
}

var currentTime = time.Now

// Normalize normalizes the profile by:
//   - Removing all duplicate samples (summing their values).
//   - Removing redundant profile labels (byte => unique of an allocation site)
//     todo: We should reassess if this was a good choice because by merging duplicate stacktrace samples
//     we cannot recompute the allocation per site ("bytes") profile label.
//   - Removing empty samples.
//   - Then remove unused references.
//   - Ensure that the profile has a time_nanos set
//   - Removes addresses from symbolized profiles.
func (p *Profile) Normalize() {
	// if the profile has no time, set it to now
	if p.TimeNanos == 0 {
		p.TimeNanos = currentTime().UnixNano()
	}

	p.ensureHasMapping()
	p.clearAddresses()

	// Non-string labels are not supported.
	for _, sample := range p.Sample {
		sample.Label = slices.RemoveInPlace(sample.Label, func(label *profilev1.Label, i int) bool {
			return label.Str == 0
		})
	}

	// first we sort the samples.
	hashes := p.hasher.Hashes(p.Sample)
	ss := &sortedSample{samples: p.Sample, hashes: hashes}
	sort.Sort(ss)
	p.Sample = ss.samples
	hashes = ss.hashes

	// Remove samples.
	var removedSamples []*profilev1.Sample

	p.Sample = slices.RemoveInPlace(p.Sample, func(s *profilev1.Sample, i int) bool {
		// if the next sample has the same hash and labels, we can remove this sample but add the value to the next sample.
		if i < len(p.Sample)-1 && hashes[i] == hashes[i+1] {
			// todo handle hashes collisions
			for j := 0; j < len(s.Value); j++ {
				p.Sample[i+1].Value[j] += s.Value[j]
			}
			removedSamples = append(removedSamples, s)
			return true
		}
		for j := 0; j < len(s.Value); j++ {
			if s.Value[j] > 0 {
				// we found a non-zero value, so we can keep this sample.
				return false
			}
		}
		// all values are 0, remove the sample.
		removedSamples = append(removedSamples, s)
		return true
	})

	// Remove references to removed samples.
	p.clearSampleReferences(removedSamples)
}

// Removes addresses from symbolized profiles.
func (p *Profile) clearAddresses() {
	for _, m := range p.Mapping {
		if m.HasFunctions {
			m.MemoryLimit = 0
			m.FileOffset = 0
			m.MemoryStart = 0
		}
	}
	for _, l := range p.Location {
		if p.Mapping[l.MappingId-1].HasFunctions {
			l.Address = 0
		}
	}
}

// ensureHasMapping ensures all locations have at least a mapping.
func (p *Profile) ensureHasMapping() {
	var mId uint64
	for _, m := range p.Mapping {
		if mId < m.Id {
			mId = m.Id
		}
	}
	var fake *profilev1.Mapping
	for _, l := range p.Location {
		if l.MappingId == 0 {
			if fake == nil {
				fake = &profilev1.Mapping{
					Id:          mId + 1,
					MemoryLimit: ^uint64(0),
				}
				p.Mapping = append(p.Mapping, fake)
			}
			l.MappingId = fake.Id
		}
	}
}

func (p *Profile) clearSampleReferences(samples []*profilev1.Sample) {
	if len(samples) == 0 {
		return
	}
	// remove all data not used anymore.
	removedLocationIds := map[uint64]struct{}{}

	for _, s := range samples {
		for _, l := range s.LocationId {
			removedLocationIds[l] = struct{}{}
		}
	}

	// figure which removed Locations IDs are not used.
	for _, s := range p.Sample {
		for _, l := range s.LocationId {
			delete(removedLocationIds, l)
		}
	}
	if len(removedLocationIds) == 0 {
		return
	}
	removedFunctionIds := map[uint64]struct{}{}
	// remove the locations that are not used anymore.
	p.Location = slices.RemoveInPlace(p.Location, func(loc *profilev1.Location, _ int) bool {
		if _, ok := removedLocationIds[loc.Id]; ok {
			for _, l := range loc.Line {
				removedFunctionIds[l.FunctionId] = struct{}{}
			}
			return true
		}
		return false
	})

	if len(removedFunctionIds) == 0 {
		return
	}
	// figure which removed Function IDs are not used.
	for _, l := range p.Location {
		for _, f := range l.Line {
			// 	// that ID is used in another location, remove it.
			delete(removedFunctionIds, f.FunctionId)
		}
	}
	removedNamesMap := map[int64]struct{}{}
	// remove the functions that are not used anymore.
	p.Function = slices.RemoveInPlace(p.Function, func(fn *profilev1.Function, _ int) bool {
		if _, ok := removedFunctionIds[fn.Id]; ok {
			removedNamesMap[fn.Name] = struct{}{}
			removedNamesMap[fn.SystemName] = struct{}{}
			removedNamesMap[fn.Filename] = struct{}{}
			return true
		}
		return false
	})

	if len(removedNamesMap) == 0 {
		return
	}
	// remove names that are still used.
	p.visitAllNameReferences(func(idx *int64) {
		delete(removedNamesMap, *idx)
	})
	if len(removedNamesMap) == 0 {
		return
	}

	// remove the names that are not used anymore.
	p.StringTable = lo.Reject(p.StringTable, func(_ string, i int) bool {
		_, ok := removedNamesMap[int64(i)]
		return ok
	})
	removedNames := lo.Keys(removedNamesMap)
	// Sort to remove in order.
	sort.Slice(removedNames, func(i, j int) bool { return removedNames[i] < removedNames[j] })
	// Now shift all indices [0,1,2,3,4,5,6]
	// if we removed [1,2,5] then we need to shift [3,4] to [1,2] and [6] to [3]
	// Basically we need to shift all indices that are greater than the removed index by the amount of removed indices.
	p.visitAllNameReferences(func(idx *int64) {
		var shift int64
		for i := 0; i < len(removedNames); i++ {
			if *idx > removedNames[i] {
				shift++
				continue
			}
			break
		}
		*idx -= shift
	})
}

func (p *Profile) visitAllNameReferences(fn func(*int64)) {
	fn(&p.DropFrames)
	fn(&p.KeepFrames)
	fn(&p.PeriodType.Type)
	fn(&p.PeriodType.Unit)
	for _, st := range p.SampleType {
		fn(&st.Type)
		fn(&st.Unit)
	}
	for _, m := range p.Mapping {
		fn(&m.Filename)
		fn(&m.BuildId)
	}
	for _, s := range p.Sample {
		for _, l := range s.Label {
			fn(&l.Key)
			fn(&l.Num)
			fn(&l.NumUnit)
		}
	}
	for _, f := range p.Function {
		fn(&f.Name)
		fn(&f.SystemName)
		fn(&f.Filename)
	}
	for i := 0; i < len(p.Comment); i++ {
		fn(&p.Comment[i])
	}
}

type SampleHasher struct {
	hash *xxhash.Digest
	b    [8]byte
}

func (h SampleHasher) Hashes(samples []*profilev1.Sample) []uint64 {
	if h.hash == nil {
		h.hash = xxhash.New()
	} else {
		h.hash.Reset()
	}

	hashes := make([]uint64, len(samples))
	for i, sample := range samples {
		if _, err := h.hash.Write(uint64Bytes(sample.LocationId)); err != nil {
			panic("unable to write hash")
		}
		sort.Sort(LabelsByKeyValue(sample.Label))
		for _, l := range sample.Label {
			binary.LittleEndian.PutUint32(h.b[:4], uint32(l.Key))
			binary.LittleEndian.PutUint32(h.b[4:], uint32(l.Str))
			if _, err := h.hash.Write(h.b[:]); err != nil {
				panic("unable to write label hash")
			}
		}
		hashes[i] = h.hash.Sum64()
		h.hash.Reset()
	}

	return hashes
}

func uint64Bytes(s []uint64) []byte {
	if len(s) == 0 {
		return nil
	}
	var bs []byte
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&bs))
	hdr.Len = len(s) * 8
	hdr.Cap = hdr.Len
	hdr.Data = uintptr(unsafe.Pointer(&s[0]))
	return bs
}

type SamplesByLabels []*profilev1.Sample

func (s SamplesByLabels) Len() int {
	return len(s)
}

func (s SamplesByLabels) Less(i, j int) bool {
	return CompareSampleLabels(s[i].Label, s[j].Label) < 0
}

func (s SamplesByLabels) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

type LabelsByKeyValue []*profilev1.Label

func (l LabelsByKeyValue) Len() int {
	return len(l)
}

func (l LabelsByKeyValue) Less(i, j int) bool {
	a, b := l[i], l[j]
	if a.Key == b.Key {
		return a.Str < b.Str
	}
	return a.Key < b.Key
}

func (l LabelsByKeyValue) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

type SampleGroup struct {
	Labels  []*profilev1.Label
	Samples []*profilev1.Sample
}

// GroupSamplesByLabels splits samples into groups by labels.
// It's expected that sample labels are sorted.
func GroupSamplesByLabels(p *profilev1.Profile) []SampleGroup {
	if len(p.Sample) < 1 {
		return nil
	}
	var result []SampleGroup
	var start int
	labels := p.Sample[start].Label
	for i := 1; i < len(p.Sample); i++ {
		if CompareSampleLabels(p.Sample[i].Label, labels) != 0 {
			result = append(result, SampleGroup{
				Labels:  labels,
				Samples: p.Sample[start:i],
			})
			start = i
			labels = p.Sample[i].Label
		}
	}
	return append(result, SampleGroup{
		Labels:  labels,
		Samples: p.Sample[start:],
	})
}

// GroupSamplesWithoutLabels splits samples into groups by labels
// ignoring ones from the list: those are preserved as sample labels.
// It's expected that sample labels are sorted.
func GroupSamplesWithoutLabels(p *profilev1.Profile, labels ...string) []SampleGroup {
	if len(labels) > 0 {
		return GroupSamplesWithoutLabelsByKey(p, LabelKeysByString(p, labels...))
	}
	return GroupSamplesByLabels(p)
}

func GroupSamplesWithoutLabelsByKey(p *profilev1.Profile, keys []int64) []SampleGroup {
	if len(p.Sample) == 0 {
		return nil
	}
	for _, s := range p.Sample {
		sort.Sort(LabelsByKeyValue(s.Label))
		// We hide labels matching the keys to the end
		// of the slice, after len() boundary.
		s.Label = LabelsWithout(s.Label, keys)
	}
	// Sorting and grouping accounts only for labels kept.
	sort.Sort(SamplesByLabels(p.Sample))
	groups := GroupSamplesByLabels(p)
	for _, s := range p.Sample {
		// Replace the labels (that match the group name)
		// with hidden labels matching the keys.
		s.Label = restoreRemovedLabels(s.Label)
	}
	return groups
}

func restoreRemovedLabels(labels []*profilev1.Label) []*profilev1.Label {
	labels = labels[len(labels):cap(labels)]
	for i, l := range labels {
		if l == nil {
			labels = labels[:i]
			break
		}
	}
	return labels
}

// CompareSampleLabels compares sample label pairs.
// It's expected that sample labels are sorted.
// The result will be 0 if a == b, < 0 if a < b, and > 0 if a > b.
func CompareSampleLabels(a, b []*profilev1.Label) int {
	l := len(a)
	if len(b) < l {
		l = len(b)
	}
	for i := 0; i < l; i++ {
		if a[i].Key != b[i].Key {
			if a[i].Key < b[i].Key {
				return -1
			}
			return 1
		}
		if a[i].Str != b[i].Str {
			if a[i].Str < b[i].Str {
				return -1
			}
			return 1
		}
	}
	return len(a) - len(b)
}

func LabelsWithout(labels []*profilev1.Label, keys []int64) []*profilev1.Label {
	n := FilterLabelsInPlace(labels, keys)
	slices.Reverse(labels) // TODO: Find a way to avoid this.
	return labels[:len(labels)-n]
}

func FilterLabelsInPlace(labels []*profilev1.Label, keys []int64) int {
	boundaryIdx := 0
	i := 0 // Pointer to labels
	j := 0 // Pointer to keys
	for i < len(labels) && j < len(keys) {
		if labels[i].Key == keys[j] {
			// If label key matches a key in keys, swap and increment both pointers
			labels[i], labels[boundaryIdx] = labels[boundaryIdx], labels[i]
			boundaryIdx++
			i++
		} else if labels[i].Key < keys[j] {
			i++ // Advance label pointer.
		} else {
			j++ // Advance key pointer.
		}
	}
	return boundaryIdx
}

func LabelKeysByString(p *profilev1.Profile, keys ...string) []int64 {
	m := LabelKeysMapByString(p, keys...)
	s := make([]int64, len(keys))
	for i, k := range keys {
		s[i] = m[k]
	}
	sort.Slice(s, func(i, j int) bool {
		return s[i] < s[j]
	})
	return s
}

func LabelKeysMapByString(p *profilev1.Profile, keys ...string) map[string]int64 {
	m := make(map[string]int64, len(keys))
	for _, k := range keys {
		m[k] = 0
	}
	for i, v := range p.StringTable {
		if _, ok := m[v]; ok {
			m[v] = int64(i)
		}
	}
	return m
}

type SampleExporter struct {
	profile *profilev1.Profile

	locations lookupTable
	functions lookupTable
	mappings  lookupTable
	strings   lookupTable
}

type lookupTable struct {
	indices  []int32
	resolved int32
}

func (t *lookupTable) lookupString(idx int64) int64 {
	if idx != 0 {
		return int64(t.lookup(idx))
	}
	return 0
}

func (t *lookupTable) lookup(idx int64) int32 {
	x := t.indices[idx]
	if x != 0 {
		return x
	}
	t.resolved++
	t.indices[idx] = t.resolved
	return t.resolved
}

func (t *lookupTable) reset() {
	t.resolved = 0
	for i := 0; i < len(t.indices); i++ {
		t.indices[i] = 0
	}
}

func NewSampleExporter(p *profilev1.Profile) *SampleExporter {
	return &SampleExporter{
		profile:   p,
		locations: lookupTable{indices: make([]int32, len(p.Location))},
		functions: lookupTable{indices: make([]int32, len(p.Function))},
		mappings:  lookupTable{indices: make([]int32, len(p.Mapping))},
		strings:   lookupTable{indices: make([]int32, len(p.StringTable))},
	}
}

// ExportSamples creates a new complete profile with the subset
// of samples provided. It is assumed that those are part of the
// source profile. Provided samples are modified in place.
//
// The same exporter instance can be used to export non-overlapping
// sample sets from a single profile.
func (e *SampleExporter) ExportSamples(dst *profilev1.Profile, samples []*profilev1.Sample) *profilev1.Profile {
	e.reset()

	dst.Sample = samples
	dst.TimeNanos = e.profile.TimeNanos
	dst.DurationNanos = e.profile.DurationNanos
	dst.Period = e.profile.Period
	dst.DefaultSampleType = e.profile.DefaultSampleType

	dst.SampleType = slices.GrowLen(dst.SampleType, len(e.profile.SampleType))
	for i, v := range e.profile.SampleType {
		dst.SampleType[i] = &profilev1.ValueType{
			Type: e.strings.lookupString(v.Type),
			Unit: e.strings.lookupString(v.Unit),
		}
	}
	dst.DropFrames = e.strings.lookupString(e.profile.DropFrames)
	dst.KeepFrames = e.strings.lookupString(e.profile.KeepFrames)
	if c := len(e.profile.Comment); c > 0 {
		dst.Comment = slices.GrowLen(dst.Comment, c)
		for i, comment := range e.profile.Comment {
			dst.Comment[i] = e.strings.lookupString(comment)
		}
	}

	// Rewrite sample stack traces and labels.
	// Note that the provided samples are modified in-place.
	for _, sample := range dst.Sample {
		for i, location := range sample.LocationId {
			sample.LocationId[i] = uint64(e.locations.lookup(int64(location - 1)))
		}
		for _, label := range sample.Label {
			label.Key = e.strings.lookupString(label.Key)
			if label.Str != 0 {
				label.Str = e.strings.lookupString(label.Str)
			} else {
				label.NumUnit = e.strings.lookupString(label.NumUnit)
			}
		}
	}

	// Copy locations.
	dst.Location = slices.GrowLen(dst.Location, int(e.locations.resolved))
	for i, j := range e.locations.indices {
		// i points to the location in the source profile.
		// j point to the location in the new profile.
		if j == 0 {
			// The location is not referenced by any of the samples.
			continue
		}
		loc := e.profile.Location[i]
		newLoc := &profilev1.Location{
			Id:        uint64(j),
			MappingId: uint64(e.mappings.lookup(int64(loc.MappingId - 1))),
			Address:   loc.Address,
			Line:      make([]*profilev1.Line, len(loc.Line)),
			IsFolded:  loc.IsFolded,
		}
		dst.Location[j-1] = newLoc
		for l, line := range loc.Line {
			newLoc.Line[l] = &profilev1.Line{
				FunctionId: uint64(e.functions.lookup(int64(line.FunctionId - 1))),
				Line:       line.Line,
			}
		}
	}

	// Copy mappings.
	dst.Mapping = slices.GrowLen(dst.Mapping, int(e.mappings.resolved))
	for i, j := range e.mappings.indices {
		if j == 0 {
			continue
		}
		m := e.profile.Mapping[i]
		dst.Mapping[j-1] = &profilev1.Mapping{
			Id:              uint64(j),
			MemoryStart:     m.MemoryStart,
			MemoryLimit:     m.MemoryLimit,
			FileOffset:      m.FileOffset,
			Filename:        e.strings.lookupString(m.Filename),
			BuildId:         e.strings.lookupString(m.BuildId),
			HasFunctions:    m.HasFunctions,
			HasFilenames:    m.HasFilenames,
			HasLineNumbers:  m.HasLineNumbers,
			HasInlineFrames: m.HasInlineFrames,
		}
	}

	// Copy functions.
	dst.Function = slices.GrowLen(dst.Function, int(e.functions.resolved))
	for i, j := range e.functions.indices {
		if j == 0 {
			continue
		}
		fn := e.profile.Function[i]
		dst.Function[j-1] = &profilev1.Function{
			Id:         uint64(j),
			Name:       e.strings.lookupString(fn.Name),
			SystemName: e.strings.lookupString(fn.SystemName),
			Filename:   e.strings.lookupString(fn.Filename),
			StartLine:  fn.StartLine,
		}
	}

	if e.profile.PeriodType != nil {
		dst.PeriodType = &profilev1.ValueType{
			Type: e.strings.lookupString(e.profile.PeriodType.Type),
			Unit: e.strings.lookupString(e.profile.PeriodType.Unit),
		}
	}

	// Copy strings.
	dst.StringTable = slices.GrowLen(dst.StringTable, int(e.strings.resolved)+1)
	for i, j := range e.strings.indices {
		if j == 0 {
			continue
		}
		dst.StringTable[j] = e.profile.StringTable[i]
	}

	return dst
}

func (e *SampleExporter) reset() {
	e.locations.reset()
	e.functions.reset()
	e.mappings.reset()
	e.strings.reset()
}

var uint32SlicePool zeropool.Pool[[]uint32]

const (
	ProfileIDLabelName = "profile_id" // For compatibility with the existing clients.
	SpanIDLabelName    = "span_id"    // Will be supported in the future.
)

func LabelID(p *profilev1.Profile, name string) int64 {
	for i, s := range p.StringTable {
		if s == name {
			return int64(i)
		}
	}
	return -1
}

func ProfileSpans(p *profilev1.Profile) []uint64 {
	if i := LabelID(p, SpanIDLabelName); i > 0 {
		return profileSpans(i, p)
	}
	return nil
}

func profileSpans(spanIDLabelIdx int64, p *profilev1.Profile) []uint64 {
	tmp := make([]byte, 8)
	s := make([]uint64, len(p.Sample))
	for i, sample := range p.Sample {
		s[i] = spanIDFromLabels(tmp, spanIDLabelIdx, p.StringTable, sample.Label)
	}
	return s
}

func spanIDFromLabels(tmp []byte, labelIdx int64, stringTable []string, labels []*profilev1.Label) uint64 {
	for _, x := range labels {
		if x.Key != labelIdx {
			continue
		}
		if s := stringTable[x.Str]; decodeSpanID(tmp, s) {
			return binary.LittleEndian.Uint64(tmp)
		}
	}
	return 0
}

func decodeSpanID(tmp []byte, s string) bool {
	if len(s) != 16 {
		return false
	}
	_, err := hex.Decode(tmp, util.YoloBuf(s))
	return err == nil
}

func RenameLabel(p *profilev1.Profile, oldName, newName string) {
	var oi, ni int64
	for i, s := range p.StringTable {
		if s == oldName {
			oi = int64(i)
			break
		}
	}
	if oi == 0 {
		return
	}
	for i, s := range p.StringTable {
		if s == newName {
			ni = int64(i)
			break
		}
	}
	if ni == 0 {
		ni = int64(len(p.StringTable))
		p.StringTable = append(p.StringTable, newName)
	}
	for _, s := range p.Sample {
		for _, l := range s.Label {
			if l.Key == oi {
				l.Key = ni
			}
		}
	}
}

func ZeroLabelStrings(p *profilev1.Profile) {
	// TODO: A true bitmap should be used instead.
	st := slices.GrowLen(uint32SlicePool.Get(), len(p.StringTable))
	slices.Clear(st)
	defer uint32SlicePool.Put(st)
	for _, t := range p.SampleType {
		st[t.Type] = 1
		st[t.Unit] = 1
	}
	for _, f := range p.Function {
		st[f.Filename] = 1
		st[f.SystemName] = 1
		st[f.Name] = 1
	}
	for _, m := range p.Mapping {
		st[m.Filename] = 1
		st[m.BuildId] = 1
	}
	for _, c := range p.Comment {
		st[c] = 1
	}
	st[p.KeepFrames] = 1
	st[p.DropFrames] = 1
	var zeroString string
	for i, v := range st {
		if v == 0 {
			p.StringTable[i] = zeroString
		}
	}
}

var languageMatchers = map[string][]string{
	"go":     {".go", "/usr/local/go/"},
	"java":   {"java/", "sun/"},
	"ruby":   {".rb", "gems/"},
	"nodejs": {"./node_modules/", ".js"},
	"dotnet": {"System.", "Microsoft."},
	"python": {".py"},
	"rust":   {"main.rs", "core.rs"},
}

func GetLanguage(profile *Profile, logger log.Logger) string {
	for _, symbol := range profile.StringTable {
		for lang, matcherPatterns := range languageMatchers {
			for _, pattern := range matcherPatterns {
				if strings.HasPrefix(symbol, pattern) || strings.HasSuffix(symbol, pattern) {
					level.Debug(logger).Log("msg", "found profile language", "lang", lang, "symbol", symbol)
					return lang
				}
			}
		}
	}
	return "unknown"
}

// SetProfileMetadata sets the metadata on the profile.
func SetProfileMetadata(p *profilev1.Profile, ty *typesv1.ProfileType, timeNanos int64, period int64) {
	m := map[string]int64{
		ty.SampleUnit: 0,
		ty.SampleType: 0,
		ty.PeriodType: 0,
		ty.PeriodUnit: 0,
	}
	for i, s := range p.StringTable {
		if _, ok := m[s]; !ok {
			m[s] = int64(i)
		}
	}
	for k, v := range m {
		if v == 0 {
			i := int64(len(p.StringTable))
			p.StringTable = append(p.StringTable, k)
			m[k] = i
		}
	}

	p.SampleType = []*profilev1.ValueType{{Type: m[ty.SampleType], Unit: m[ty.SampleUnit]}}
	p.DefaultSampleType = m[ty.SampleType]
	p.PeriodType = &profilev1.ValueType{Type: m[ty.PeriodType], Unit: m[ty.PeriodUnit]}
	p.TimeNanos = timeNanos

	if period != 0 {
		p.Period = period
	}

	// Try to guess period based on the profile type.
	// TODO: This should be encoded into the profile type.
	switch ty.Name {
	case "process_cpu":
		p.Period = 1000000000
	case "memory":
		p.Period = 512 * 1024
	default:
		p.Period = 1
	}
}

func Marshal(p *profilev1.Profile, compress bool) ([]byte, error) {
	b, err := p.MarshalVT()
	if err != nil {
		return nil, err
	}
	if !compress {
		return b, nil
	}
	var buf bytes.Buffer
	buf.Grow(len(b) / 2)
	gw := gzipWriterPool.Get().(*gzip.Writer)
	gw.Reset(&buf)
	defer func() {
		gw.Reset(io.Discard)
		gzipWriterPool.Put(gw)
	}()
	if _, err = gw.Write(b); err != nil {
		return nil, err
	}
	if err = gw.Flush(); err != nil {
		return nil, err
	}
	if err = gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Unmarshal(data []byte, p *profilev1.Profile) error {
	gr := gzipReaderPool.Get().(*gzipReader)
	defer gzipReaderPool.Put(gr)
	r, err := gr.openBytes(data)
	if err != nil {
		return err
	}
	buf := bufPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		bufPool.Put(buf)
	}()
	buf.Grow(len(data) * 2)
	if _, err = io.Copy(buf, r); err != nil {
		return err
	}
	return p.UnmarshalVT(buf.Bytes())
}
