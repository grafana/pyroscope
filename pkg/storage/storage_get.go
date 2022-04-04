package storage

import (
	"context"
	"fmt"
	"math/big"
	"runtime/trace"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type GetInput struct {
	StartTime time.Time
	EndTime   time.Time
	Key       *segment.Key
	Query     *flameql.Query
}

type GetOutput struct {
	Tree       *tree.Tree
	Timeline   *segment.Timeline
	SpyName    string
	SampleRate uint32
	Units      string
}

const (
	averageAggregationType = "average"

	traceTaskGet        = "storage.Get"
	traceCatGetKey      = traceTaskGet
	traceCatGetCallback = traceTaskGet + ".Callback"
)

func (s *Storage) Get(ctx context.Context, gi *GetInput) (*GetOutput, error) {
	var t *trace.Task
	ctx, t = trace.NewTask(ctx, traceTaskGet)
	defer t.End()
	logger := logrus.WithFields(logrus.Fields{
		"startTime": gi.StartTime.String(),
		"endTime":   gi.EndTime.String(),
	})

	var dimensionKeys func() []dimension.Key
	switch {
	case gi.Key != nil:
		logger = logger.WithField("key", gi.Key.Normalized())
		dimensionKeys = s.dimensionKeysByKey(gi.Key)
	case gi.Query != nil:
		logger = logger.WithField("query", gi.Query)
		dimensionKeys = s.dimensionKeysByQuery(ctx, gi.Query)
	default:
		// Should never happen.
		return nil, fmt.Errorf("key or query must be specified")
	}

	s.getTotal.Inc()
	logger.Debug("storage.Get")
	trace.Logf(ctx, traceCatGetKey, "%+v", gi)

	// For backward compatibility, profiles can be fetched by ID using query.
	// If a query includes 'profile_id' matcher others are ignored.
	if gi.Query != nil {
		ids := make([]string, 0, len(gi.Query.Matchers))
		for _, m := range gi.Query.Matchers {
			if m.Key != segment.ProfileIDLabelName {
				continue
			}
			if m.Op != flameql.OpEqual {
				return nil, fmt.Errorf("only '=' operator is allowed for %q label", segment.ProfileIDLabelName)
			}
			ids = append(ids, m.Value)
		}
		if len(ids) > 0 {
			o := GetOutput{
				SpyName:    "gospy",
				Units:      "samples",
				SampleRate: 100,
				Tree:       tree.New(),
			}
			err := s.profiles.fetch(ctx, gi.Query.AppName, ids, func(t *tree.Tree) error {
				o.Tree.Merge(t)
				return nil
			})
			if err != nil {
				return nil, err
			}
			return &o, nil
		}
	}

	var (
		resultTrie  *tree.Tree
		lastSegment *segment.Segment
		writesTotal uint64

		aggregationType = "sum"
		timeline        = segment.GenerateTimeline(gi.StartTime, gi.EndTime)
	)

	for _, k := range dimensionKeys() {
		// TODO: refactor, store `Key`s in dimensions
		parsedKey, err := segment.ParseKey(string(k))
		if err != nil {
			s.logger.Errorf("parse key: %v: %v", string(k), err)
			continue
		}
		key := parsedKey.SegmentKey()
		res, ok := s.segments.Lookup(key)
		if !ok {
			continue
		}

		st := res.(*segment.Segment)
		if st.AggregationType() == averageAggregationType {
			aggregationType = averageAggregationType
		}

		timeline.PopulateTimeline(st)
		lastSegment = st

		trace.Logf(ctx, traceCatGetCallback, "segment_key=%s", key)
		st.GetContext(ctx, gi.StartTime, gi.EndTime, func(depth int, samples, writes uint64, t time.Time, r *big.Rat) {
			tk := parsedKey.TreeKey(depth, t)
			res, ok = s.trees.Lookup(tk)
			trace.Logf(ctx, traceCatGetCallback, "tree_found=%v time=%d r=%v", ok, t.Unix(), r)
			if ok {
				x := res.(*tree.Tree).Clone(r)
				writesTotal += writes
				if resultTrie == nil {
					resultTrie = x
					return
				}
				resultTrie.Merge(x)
			}
		})
	}

	if resultTrie == nil || lastSegment == nil {
		return nil, nil
	}

	if writesTotal > 0 && aggregationType == averageAggregationType {
		resultTrie = resultTrie.Clone(big.NewRat(1, int64(writesTotal)))
	}

	return &GetOutput{
		Tree:       resultTrie,
		Timeline:   timeline,
		SpyName:    lastSegment.SpyName(),
		SampleRate: lastSegment.SampleRate(),
		Units:      lastSegment.Units(),
	}, nil
}

func (s *Storage) execQuery(_ context.Context, qry *flameql.Query) []dimension.Key {
	app, found := s.lookupAppDimension(qry.AppName)
	if !found {
		return nil
	}
	if len(qry.Matchers) == 0 {
		return app.Keys
	}

	r := []*dimension.Dimension{app}
	var n []*dimension.Dimension // Keys to be removed from the result.

	for _, m := range qry.Matchers {
		switch m.Op {
		case flameql.OpEqual:
			if d, ok := s.lookupDimension(m); ok {
				r = append(r, d)
			} else {
				return nil
			}
		case flameql.OpNotEqual:
			if d, ok := s.lookupDimension(m); ok {
				n = append(n, d)
			}
		case flameql.OpEqualRegex:
			if d, ok := s.lookupDimensionRegex(m); ok {
				r = append(r, d)
			} else {
				return nil
			}
		case flameql.OpNotEqualRegex:
			if d, ok := s.lookupDimensionRegex(m); ok {
				n = append(n, d)
			}
		}
	}

	i := dimension.Intersection(r...)
	if len(n) == 0 {
		return i
	}

	return dimension.AndNot(
		&dimension.Dimension{Keys: i},
		&dimension.Dimension{Keys: dimension.Union(n...)})
}

func (s *Storage) dimensionKeysByQuery(ctx context.Context, qry *flameql.Query) func() []dimension.Key {
	return func() []dimension.Key { return s.execQuery(ctx, qry) }
}

func (s *Storage) dimensionKeysByKey(key *segment.Key) func() []dimension.Key {
	return func() []dimension.Key {
		d, ok := s.lookupAppDimension(key.AppName())
		if !ok {
			return nil
		}
		l := key.Labels()
		if len(l) == 1 {
			// No tags specified: return application dimension keys.
			return d.Keys
		}
		dimensions := []*dimension.Dimension{d}
		for k, v := range l {
			if flameql.IsTagKeyReserved(k) {
				continue
			}
			if d, ok = s.lookupDimensionKV(k, v); ok {
				dimensions = append(dimensions, d)
			}
		}
		if len(dimensions) == 1 {
			// Tags specified but not found.
			return nil
		}
		return dimension.Intersection(dimensions...)
	}
}

func (s *Storage) lookupAppDimension(app string) (*dimension.Dimension, bool) {
	return s.lookupDimensionKV("__name__", app)
}

func (s *Storage) lookupDimension(m *flameql.TagMatcher) (*dimension.Dimension, bool) {
	return s.lookupDimensionKV(m.Key, m.Value)
}

func (s *Storage) lookupDimensionRegex(m *flameql.TagMatcher) (*dimension.Dimension, bool) {
	d := dimension.New()
	s.labels.GetValues(m.Key, func(v string) bool {
		if m.R.MatchString(v) {
			if x, ok := s.lookupDimensionKV(m.Key, v); ok {
				d.Keys = append(d.Keys, x.Keys...)
			}
		}
		return true
	})
	if len(d.Keys) > 0 {
		return d, true
	}
	return nil, false
}

func (s *Storage) lookupDimensionKV(k, v string) (*dimension.Dimension, bool) {
	r, ok := s.dimensions.Lookup(k + ":" + v)
	if ok {
		return r.(*dimension.Dimension), true
	}
	return nil, false
}
