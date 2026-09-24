package querybackend

import (
	"sync"

	"github.com/grafana/dskit/runutil"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_PROFILE_PRESENCE,
		queryv1.ReportType_REPORT_PROFILE_PRESENCE,
		queryProfilePresence,
		newProfilePresenceAggregator,
		false,
		[]block.Section{
			block.SectionTSDB,
			block.SectionProfiles,
		}...,
	)
}

// queryProfilePresence checks which of query.ProfilePresence.ProfileIdSelector are present in
// this block (matching the label selector and time range already applied via q.req.matchers /
// q.req.startTime|endTime). Unlike queryTree/queryPprof, it never resolves symbols or merges a
// profile -- it only reads the ID column -- and it checks the entire candidate ID list in one
// pass rather than needing one query per candidate.
//
// withExcludeSampled() matches queryTree/queryPprof/queryHeatmap: profiles labeled
// __sampled__="true" are stripped/reduced-fidelity samples that never resolve to a real
// flame graph (see query_time_series.go's "stripped" check). A caller of this query type wants
// to know "would a real profile view for this ID actually show something", so a stripped
// profile must count as absent, not present -- otherwise this would confirm anomalies whose
// resulting Grafana link renders empty, which is the exact problem this query type exists to
// avoid.
func queryProfilePresence(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	opt, err := withProfileIDSelector(query.ProfilePresence.ProfileIdSelector...)
	if err != nil {
		return nil, err
	}

	// withAllLabels() is needed to actually populate entry.Labels below -- without it, the
	// iterator doesn't bother resolving the series' labels at all, since none of the other
	// query types built on profileEntryIterator need them for anything but joining/matching.
	entries, err := profileEntryIterator(q, opt, withExcludeSampled(), withAllLabels())
	if err != nil {
		return nil, err
	}
	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")

	seen := make(map[string]phlaremodel.Labels)
	for entries.Next() {
		e := entries.At()
		seen[e.ID] = e.Labels
	}
	if err = entries.Err(); err != nil {
		return nil, err
	}

	profiles := make([]*queryv1.ProfilePresenceEntry, 0, len(seen))
	for id, lbls := range seen {
		profiles = append(profiles, &queryv1.ProfilePresenceEntry{ProfileId: id, Labels: lbls})
	}
	return &queryv1.Report{
		ProfilePresence: &queryv1.ProfilePresenceReport{
			Query:    query.ProfilePresence.CloneVT(),
			Profiles: profiles,
		},
	}, nil
}

type profilePresenceAggregator struct {
	init  sync.Once
	query *queryv1.ProfilePresenceQuery
	seen  map[string]phlaremodel.Labels
}

func newProfilePresenceAggregator(*queryv1.InvokeRequest) aggregator {
	return new(profilePresenceAggregator)
}

func (a *profilePresenceAggregator) aggregate(report *queryv1.Report) error {
	r := report.ProfilePresence
	a.init.Do(func() {
		a.query = r.Query.CloneVT()
		a.seen = make(map[string]phlaremodel.Labels, len(r.Profiles))
	})
	for _, p := range r.Profiles {
		a.seen[p.ProfileId] = p.Labels
	}
	return nil
}

func (a *profilePresenceAggregator) build() *queryv1.Report {
	profiles := make([]*queryv1.ProfilePresenceEntry, 0, len(a.seen))
	for id, lbls := range a.seen {
		profiles = append(profiles, &queryv1.ProfilePresenceEntry{ProfileId: id, Labels: lbls})
	}
	return &queryv1.Report{
		ProfilePresence: &queryv1.ProfilePresenceReport{
			Query:    a.query,
			Profiles: profiles,
		},
	}
}
