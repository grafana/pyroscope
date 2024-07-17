package model

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/cespare/xxhash/v2"
	pmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/util"
)

var seps = []byte{'\xff'}

const (
	LabelNameProfileType        = "__profile_type__"
	LabelNameServiceNamePrivate = "__service_name__"
	LabelNameDelta              = "__delta__"
	LabelNameProfileName        = pmodel.MetricNameLabel
	LabelNamePeriodType         = "__period_type__"
	LabelNamePeriodUnit         = "__period_unit__"
	LabelNameSessionID          = "__session_id__"
	LabelNameType               = "__type__"
	LabelNameUnit               = "__unit__"

	LabelNameServiceGitRef     = "service_git_ref"
	LabelNameServiceName       = "service_name"
	LabelNameServiceRepository = "service_repository"

	LabelNameOrder     = "__order__"
	LabelOrderEnforced = "enforced"

	LabelNamePyroscopeSpy = "pyroscope_spy"

	labelSep = '\xfe'
)

// Labels is a sorted set of labels. Order has to be guaranteed upon
// instantiation.
type Labels []*typesv1.LabelPair

func (ls Labels) Len() int           { return len(ls) }
func (ls Labels) Swap(i, j int)      { ls[i], ls[j] = ls[j], ls[i] }
func (ls Labels) Less(i, j int) bool { return ls[i].Name < ls[j].Name }

// Range calls f on each label.
func (ls Labels) Range(f func(l *typesv1.LabelPair)) {
	for _, l := range ls {
		f(l)
	}
}

// EmptyLabels returns n empty Labels value, for convenience.
func EmptyLabels() Labels {
	return Labels{}
}

// LabelsEnforcedOrder is a sort order of labels, where profile type and
// service name labels always go first. This is crucial for query performance
// as labels determine the physical order of the profiling data.
type LabelsEnforcedOrder []*typesv1.LabelPair

func (ls LabelsEnforcedOrder) Len() int      { return len(ls) }
func (ls LabelsEnforcedOrder) Swap(i, j int) { ls[i], ls[j] = ls[j], ls[i] }

func (ls LabelsEnforcedOrder) Less(i, j int) bool {
	if ls[i].Name[0] == '_' || ls[j].Name[0] == '_' {
		leftType := ls[i].Name == LabelNameProfileType
		rightType := ls[j].Name == LabelNameProfileType
		if leftType || rightType {
			return leftType || !rightType
		}
		leftService := ls[i].Name == LabelNameServiceNamePrivate
		rightService := ls[j].Name == LabelNameServiceNamePrivate
		if leftService || rightService {
			return leftService || !rightService
		}
	}
	return ls[i].Name < ls[j].Name
}

// Hash returns a hash value for the label set.
func (ls Labels) Hash() uint64 {
	// Use xxhash.Sum64(b) for fast path as it's faster.
	b := make([]byte, 0, 1024)
	for i, v := range ls {
		if len(b)+len(v.Name)+len(v.Value)+2 >= cap(b) {
			// If labels entry is 1KB+ do not allocate whole entry.
			h := xxhash.New()
			_, _ = h.Write(b)
			for _, v := range ls[i:] {
				_, _ = h.WriteString(v.Name)
				_, _ = h.Write(seps)
				_, _ = h.WriteString(v.Value)
				_, _ = h.Write(seps)
			}
			return h.Sum64()
		}

		b = append(b, v.Name...)
		b = append(b, seps[0])
		b = append(b, v.Value...)
		b = append(b, seps[0])
	}
	return xxhash.Sum64(b)
}

// BytesWithLabels is just as Bytes(), but only for labels matching names.
// It uses an byte invalid character as a separator and so should not be used for printing.
func (ls Labels) BytesWithLabels(buf []byte, names ...string) []byte {
	buf = buf[:0]
	buf = append(buf, labelSep)
	for _, name := range names {
		for _, l := range ls {
			if l.Name == name {
				if len(buf) > 1 {
					buf = append(buf, seps[0])
				}
				buf = append(buf, l.Name...)
				buf = append(buf, seps[0])
				buf = append(buf, l.Value...)
				break
			}
		}
	}
	return buf
}

func (ls Labels) ToPrometheusLabels() labels.Labels {
	res := make([]labels.Label, len(ls))
	for i, l := range ls {
		res[i] = labels.Label{Name: l.Name, Value: l.Value}
	}
	return res
}

func (ls Labels) WithoutPrivateLabels() Labels {
	res := make([]*typesv1.LabelPair, 0, len(ls))
	for _, l := range ls {
		if !strings.HasPrefix(l.Name, "__") {
			res = append(res, l)
		}
	}
	return res
}

var allowedPrivateLabels = map[string]struct{}{
	LabelNameSessionID: {},
}

func IsLabelAllowedForIngestion(name string) bool {
	if !strings.HasPrefix(name, "__") {
		return true
	}
	_, allowed := allowedPrivateLabels[name]
	return allowed
}

// WithLabels returns a subset of Labels that match with the provided label names.
func (ls Labels) WithLabels(names ...string) Labels {
	matched := make(Labels, 0, len(names))
	for _, name := range names {
		for _, l := range ls {
			if l.Name == name {
				matched = append(matched, l)
				break
			}
		}
	}
	return matched
}

// Get returns the value for the label with the given name.
// Returns an empty string if the label doesn't exist.
func (ls Labels) Get(name string) string {
	for _, l := range ls {
		if l.Name == name {
			return l.Value
		}
	}
	return ""
}

// GetLabel returns the label with the given name.
func (ls Labels) GetLabel(name string) (*typesv1.LabelPair, bool) {
	for _, l := range ls {
		if l.Name == name {
			return l, true
		}
	}
	return nil, false
}

// Delete removes the first label encountered with the name given in place.
func (ls Labels) Delete(name string) Labels {
	for i, l := range ls {
		if l.Name == name {
			return slices.Delete(ls, i, i+1)
		}
	}
	return ls
}

// InsertSorted adds the given label to the set of labels.
// It assumes the labels are sorted lexicographically.
func (ls Labels) InsertSorted(name, value string) Labels {
	// Find the index where the new label should be inserted.
	// TODO: Use binary search on large label sets.
	index := -1
	for i, label := range ls {
		if label.Name > name {
			index = i
			break
		}
		if label.Name == name {
			label.Value = value
			return ls
		}
	}
	// Insert the new label at the found index.
	l := &typesv1.LabelPair{
		Name:  name,
		Value: value,
	}
	c := append(ls, l)
	if index == -1 {
		return c
	}
	copy((c)[index+1:], (c)[index:])
	(c)[index] = l
	return c
}

func (ls Labels) Clone() Labels {
	result := make(Labels, len(ls))
	for i, l := range ls {
		result[i] = &typesv1.LabelPair{
			Name:  l.Name,
			Value: l.Value,
		}
	}
	return result
}

// Unique returns a set labels with unique keys.
// Labels expected to be sorted: underlying capacity
// is reused and the original order is preserved:
// the first key takes precedence over duplicates.
// Method receiver should not be used after the call.
func (ls Labels) Unique() Labels {
	if len(ls) <= 1 {
		return ls
	}
	var j int
	for i := 1; i < len(ls); i++ {
		if ls[i].Name != ls[j].Name {
			j++
			ls[j] = ls[i]
		}
	}
	return ls[:j+1]
}

// LabelPairsString returns a string representation of the label pairs.
func LabelPairsString(lbs []*typesv1.LabelPair) string {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, l := range lbs {
		if i > 0 {
			b.WriteByte(',')
			b.WriteByte(' ')
		}
		b.WriteString(l.Name)
		b.WriteByte('=')
		b.WriteString(strconv.Quote(l.Value))
	}
	b.WriteByte('}')
	return b.String()
}

// LabelsFromMap returns new sorted Labels from the given map.
func LabelsFromMap(m map[string]string) Labels {
	res := make(Labels, 0, len(m))
	for k, v := range m {
		res = append(res, &typesv1.LabelPair{Name: k, Value: v})
	}
	sort.Sort(res)
	return res
}

// LabelsFromStrings creates new labels from pairs of strings.
func LabelsFromStrings(ss ...string) Labels {
	if len(ss)%2 != 0 {
		panic("invalid number of strings")
	}
	var res Labels
	for i := 0; i < len(ss); i += 2 {
		res = append(res, &typesv1.LabelPair{Name: ss[i], Value: ss[i+1]})
	}

	sort.Sort(res)
	return res
}

// CompareLabelPairs compares the two label sets.
// The result will be 0 if a==b, <0 if a < b, and >0 if a > b.
func CompareLabelPairs(a []*typesv1.LabelPair, b []*typesv1.LabelPair) int {
	l := len(a)
	if len(b) < l {
		l = len(b)
	}

	for i := 0; i < l; i++ {
		if a[i].Name != b[i].Name {
			if a[i].Name < b[i].Name {
				return -1
			}
			return 1
		}
		if a[i].Value != b[i].Value {
			if a[i].Value < b[i].Value {
				return -1
			}
			return 1
		}
	}
	// If all labels so far were in common, the set with fewer labels comes first.
	return len(a) - len(b)
}

// LabelsBuilder allows modifying Labels.
type LabelsBuilder struct {
	base Labels
	del  []string
	add  []*typesv1.LabelPair
}

// NewLabelsBuilder returns a new LabelsBuilder.
func NewLabelsBuilder(base Labels) *LabelsBuilder {
	b := &LabelsBuilder{
		del: make([]string, 0, 5),
		add: make([]*typesv1.LabelPair, 0, 5),
	}
	b.Reset(base)
	return b
}

// Reset clears all current state for the builder.
func (b *LabelsBuilder) Reset(base Labels) {
	b.base = base
	b.del = b.del[:0]
	b.add = b.add[:0]
	for _, l := range b.base {
		if l.Value == "" {
			b.del = append(b.del, l.Name)
		}
	}
}

// Del deletes the label of the given name.
func (b *LabelsBuilder) Del(ns ...string) *LabelsBuilder {
	for _, n := range ns {
		for i, a := range b.add {
			if a.Name == n {
				b.add = append(b.add[:i], b.add[i+1:]...)
			}
		}
		b.del = append(b.del, n)
	}
	return b
}

// Set the name/value pair as a label.
func (b *LabelsBuilder) Set(n, v string) *LabelsBuilder {
	if v == "" {
		// Empty labels are the same as missing labels.
		return b.Del(n)
	}
	for i, a := range b.add {
		if a.Name == n {
			b.add[i].Value = v
			return b
		}
	}
	b.add = append(b.add, &typesv1.LabelPair{Name: n, Value: v})

	return b
}

func (b *LabelsBuilder) Get(n string) string {
	// Del() removes entries from .add but Set() does not remove from .del, so check .add first.
	for _, a := range b.add {
		if a.Name == n {
			return a.Value
		}
	}
	if slices.Contains(b.del, n) {
		return ""
	}
	return b.base.Get(n)
}

// Range calls f on each label in the Builder.
func (b *LabelsBuilder) Range(f func(l *typesv1.LabelPair)) {
	// Stack-based arrays to avoid heap allocation in most cases.
	var addStack [128]*typesv1.LabelPair
	var delStack [128]string
	// Take a copy of add and del, so they are unaffected by calls to Set() or Del().
	origAdd, origDel := append(addStack[:0], b.add...), append(delStack[:0], b.del...)
	b.base.Range(func(l *typesv1.LabelPair) {
		if !slices.Contains(origDel, l.Name) && !contains(origAdd, l.Name) {
			f(l)
		}
	})
	for _, a := range origAdd {
		f(a)
	}
}

func contains(s []*typesv1.LabelPair, n string) bool {
	for _, a := range s {
		if a.Name == n {
			return true
		}
	}
	return false
}

// Labels returns the labels from the builder. If no modifications
// were made, the original labels are returned.
func (b *LabelsBuilder) Labels() Labels {
	res := b.LabelsUnsorted()
	sort.Sort(res)
	return res
}

// LabelsUnsorted returns the labels from the builder. If no modifications
// were made, the original labels are returned.
//
// The order is not deterministic.
func (b *LabelsBuilder) LabelsUnsorted() Labels {
	if len(b.del) == 0 && len(b.add) == 0 {
		return b.base
	}

	// In the general case, labels are removed, modified or moved
	// rather than added.
	res := make(Labels, 0, len(b.base))
Outer:
	for _, l := range b.base {
		for _, n := range b.del {
			if l.Name == n {
				continue Outer
			}
		}
		for _, la := range b.add {
			if l.Name == la.Name {
				continue Outer
			}
		}
		res = append(res, l)
	}

	return append(res, b.add...)
}

type SessionID uint64

func (s SessionID) String() string {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(s))
	return hex.EncodeToString(b[:])
}

func ParseSessionID(s string) (SessionID, error) {
	if len(s) != 16 {
		return 0, fmt.Errorf("invalid session id length %d", len(s))
	}
	var b [8]byte
	if _, err := hex.Decode(b[:], util.YoloBuf(s)); err != nil {
		return 0, err
	}
	return SessionID(binary.LittleEndian.Uint64(b[:])), nil
}

type ServiceVersion struct {
	Repository string `json:"repository,omitempty"`
	GitRef     string `json:"git_ref,omitempty"`
	BuildID    string `json:"build_id,omitempty"`
}

// ServiceVersionFromLabels Attempts to extract a service version from the given labels.
// Returns false if no service version was found.
func ServiceVersionFromLabels(lbls Labels) (ServiceVersion, bool) {
	repo := lbls.Get(LabelNameServiceRepository)
	gitref := lbls.Get(LabelNameServiceGitRef)
	return ServiceVersion{
		Repository: repo,
		GitRef:     gitref,
	}, repo != "" || gitref != ""
}
