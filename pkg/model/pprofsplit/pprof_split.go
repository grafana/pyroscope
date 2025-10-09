package pprofsplit

import (
	"unsafe"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/model/relabel"
	"github.com/grafana/pyroscope/pkg/pprof"
)

type SampleSeriesVisitor interface {
	// VisitProfile is called when no sample labels are present in
	// the profile, or if all the sample labels are identical.
	// Provided labels are the series labels processed with relabeling rules.
	VisitProfile(phlaremodel.Labels)
	VisitSampleSeries(phlaremodel.Labels, []*profilev1.Sample)
	// ValidateLabels is called to validate the labels before
	// they are passed to the visitor.
	ValidateLabels(phlaremodel.Labels) error
	Discarded(profiles, bytes int)
}

func VisitSampleSeries(
	profile *profilev1.Profile,
	labels []*typesv1.LabelPair,
	rules []*relabel.Config,
	visitor SampleSeriesVisitor,
) error {
	var profilesDiscarded, bytesDiscarded int
	defer func() {
		visitor.Discarded(profilesDiscarded, bytesDiscarded)
	}()

	pprof.RenameLabel(profile, pprof.ProfileIDLabelName, pprof.SpanIDLabelName)
	groups := pprof.GroupSamplesWithoutLabels(profile, pprof.SpanIDLabelName)
	builder := phlaremodel.NewLabelsBuilder(nil)

	if len(groups) == 0 || (len(groups) == 1 && len(groups[0].Labels) == 0) {
		// No sample labels in the profile.
		// Relabel the series labels.
		builder.Reset(labels)
		if len(rules) > 0 {
			keep := relabel.ProcessBuilder(builder, rules...)
			if !keep {
				// We drop the profile.
				profilesDiscarded++
				bytesDiscarded += profile.SizeVT()
				return nil
			}
		}
		if len(profile.Sample) > 0 {
			labels = builder.Labels()
			if err := visitor.ValidateLabels(labels); err != nil {
				return err
			}
			visitor.VisitProfile(labels)
		}
		return nil
	}

	// iterate through groups relabel them and find relevant overlapping label sets.
	groupsKept := newGroupsWithFingerprints()
	for _, group := range groups {
		builder.Reset(labels)
		addSampleLabelsToLabelsBuilder(builder, profile, group.Labels)
		if len(rules) > 0 {
			keep := relabel.ProcessBuilder(builder, rules...)
			if !keep {
				bytesDiscarded += sampleSize(group.Samples)
				continue
			}
		}
		// add the group to the list.
		groupsKept.add(profile.StringTable, builder.Labels(), group)
	}

	if len(groupsKept.m) == 0 {
		// no groups kept, count the whole profile as dropped
		profilesDiscarded++
		return nil
	}

	for _, idx := range groupsKept.order {
		for _, group := range groupsKept.m[idx] {
			if len(group.sampleGroup.Samples) > 0 {
				if err := visitor.ValidateLabels(group.labels); err != nil {
					return err
				}
				visitor.VisitSampleSeries(group.labels, group.sampleGroup.Samples)
			}
		}
	}

	return nil
}

// addSampleLabelsToLabelsBuilder: adds sample label that don't exists yet on the profile builder. So the existing labels take precedence.
func addSampleLabelsToLabelsBuilder(b *phlaremodel.LabelsBuilder, p *profilev1.Profile, pl []*profilev1.Label) {
	var name string
	for _, l := range pl {
		name = p.StringTable[l.Key]
		if l.Str <= 0 {
			// skip if label value is not a string
			continue
		}
		if b.Get(name) != "" {
			// do nothing if label name already exists
			continue
		}
		b.Set(name, p.StringTable[l.Str])
	}
}

type sampleKey struct {
	stacktrace string
	// note this is an index into the string table, rather than span ID
	spanIDIdx int64
}

func sampleKeyFromSample(stringTable []string, s *profilev1.Sample) sampleKey {
	var k sampleKey
	// populate spanID if present
	for _, l := range s.Label {
		if stringTable[int(l.Key)] == pprof.SpanIDLabelName {
			k.spanIDIdx = l.Str
		}
	}
	if len(s.LocationId) > 0 {
		k.stacktrace = unsafe.String(
			(*byte)(unsafe.Pointer(&s.LocationId[0])),
			len(s.LocationId)*8,
		)
	}
	return k
}

type lazyGroup struct {
	sampleGroup pprof.SampleGroup
	// The map is only initialized when the group is being modified. Key is the
	// string representation (unsafe) of the sample stack trace and its potential
	// span ID.
	sampleMap map[sampleKey]*profilev1.Sample
	labels    phlaremodel.Labels
}

func (g *lazyGroup) addSampleGroup(stringTable []string, sg pprof.SampleGroup) {
	if len(g.sampleGroup.Samples) == 0 {
		g.sampleGroup = sg
		return
	}

	// If the group is already initialized, we need to merge the samples.
	if g.sampleMap == nil {
		g.sampleMap = make(map[sampleKey]*profilev1.Sample)
		for _, s := range g.sampleGroup.Samples {
			g.sampleMap[sampleKeyFromSample(stringTable, s)] = s
		}
	}

	for _, s := range sg.Samples {
		k := sampleKeyFromSample(stringTable, s)
		if _, ok := g.sampleMap[k]; !ok {
			g.sampleGroup.Samples = append(g.sampleGroup.Samples, s)
			g.sampleMap[k] = s
		} else {
			// merge the samples
			for idx := range s.Value {
				g.sampleMap[k].Value[idx] += s.Value[idx]
			}
		}
	}
}

type groupsWithFingerprints struct {
	m     map[uint64][]*lazyGroup
	order []uint64
}

func newGroupsWithFingerprints() *groupsWithFingerprints {
	return &groupsWithFingerprints{
		m: make(map[uint64][]*lazyGroup),
	}
}

func (g *groupsWithFingerprints) add(stringTable []string, lbls phlaremodel.Labels, group pprof.SampleGroup) {
	fp := lbls.Hash()
	idxs, ok := g.m[fp]
	if ok {
		// fingerprint matches, check if the labels are the same
		for _, idx := range idxs {
			if phlaremodel.CompareLabelPairs(idx.labels, lbls) == 0 {
				// append samples to the group
				idx.addSampleGroup(stringTable, group)
				return
			}
		}
	} else {
		g.order = append(g.order, fp)
	}

	// add the labels to the list
	g.m[fp] = append(g.m[fp], &lazyGroup{
		sampleGroup: group,
		labels:      lbls,
	})
}

// sampleSize returns the size of a samples in bytes.
func sampleSize(samples []*profilev1.Sample) int {
	var size int
	for _, s := range samples {
		size += s.SizeVT()
	}
	return size
}
