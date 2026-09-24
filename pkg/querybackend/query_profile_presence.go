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
// this block.
func queryProfilePresence(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	opts, err := profilePresenceIteratorOptions(query.ProfilePresence.ProfileIdSelector)
	if err != nil {
		return nil, err
	}

	entries, err := profileEntryIterator(q, opts...)
	if err != nil {
		return nil, err
	}
	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")

	seen := make(map[string]profilePresenceInfo)
	for entries.Next() {
		e := entries.At()
		seen[e.ID] = profilePresenceInfo{Labels: e.Labels, Timestamp: int64(e.Timestamp)}
	}
	if err = entries.Err(); err != nil {
		return nil, err
	}

	profiles := make([]*queryv1.ProfilePresenceEntry, 0, len(seen))
	for id, info := range seen {
		profiles = append(profiles, &queryv1.ProfilePresenceEntry{
			ProfileId: id,
			Labels:    info.Labels,
			Timestamp: info.Timestamp,
		})
	}
	return &queryv1.Report{
		ProfilePresence: &queryv1.ProfilePresenceReport{
			Query:    query.ProfilePresence.CloneVT(),
			Profiles: profiles,
		},
	}, nil
}

// profilePresenceIteratorOptions is covered by TestProfilePresenceIteratorOptions: excludeSampled
// and allLabels must stay set, or queryProfilePresence silently regresses (reports stripped
// profiles present; drops the labels callers read from ProfilePresenceEntry).
func profilePresenceIteratorOptions(ids []string) ([]profileIteratorOption, error) {
	opt, err := withProfileIDSelector(ids...)
	if err != nil {
		return nil, err
	}
	return []profileIteratorOption{opt, withExcludeSampled(), withAllLabels()}, nil
}

type profilePresenceInfo struct {
	Labels    phlaremodel.Labels
	Timestamp int64
}

type profilePresenceAggregator struct {
	init   sync.Once
	query  *queryv1.ProfilePresenceQuery
	merger *phlaremodel.ProfilePresenceMerger
}

func newProfilePresenceAggregator(*queryv1.InvokeRequest) aggregator {
	return new(profilePresenceAggregator)
}

func (a *profilePresenceAggregator) aggregate(report *queryv1.Report) error {
	r := report.ProfilePresence
	a.init.Do(func() {
		a.query = r.Query.CloneVT()
		a.merger = phlaremodel.NewProfilePresenceMerger()
	})
	a.merger.MergeProfilePresence(r.Profiles)
	return nil
}

func (a *profilePresenceAggregator) build() *queryv1.Report {
	return &queryv1.Report{
		ProfilePresence: &queryv1.ProfilePresenceReport{
			Query:    a.query,
			Profiles: a.merger.Profiles(),
		},
	}
}
