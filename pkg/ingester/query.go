package ingester

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"
	"github.com/bufbuild/connect-go"
	"github.com/gogo/status"
	"github.com/parca-dev/parca/pkg/metastore"
	"github.com/parca-dev/parca/pkg/parcacol"
	"github.com/polarsignals/frostdb/query/logicalplan"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"google.golang.org/grpc/codes"

	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
)

// LabelValues returns the possible label values for a given label name.
func (i *Ingester) LabelValues(ctx context.Context, req *connect.Request[ingestv1.LabelValuesRequest]) (*connect.Response[ingestv1.LabelValuesResponse], error) {
	vals := []string{}

	err := i.engine.ScanTable("stacktraces").
		Distinct(logicalplan.Col("labels."+req.Msg.Name)).
		Execute(ctx, func(ar arrow.Record) error {
			if ar.NumCols() != 1 {
				return fmt.Errorf("expected 1 column, got %d", ar.NumCols())
			}

			col := ar.Column(0)
			stringCol, ok := col.(*array.Binary)
			if !ok {
				return fmt.Errorf("expected string column, got %T", col)
			}

			for i := 0; i < stringCol.Len(); i++ {
				val := stringCol.Value(i)
				vals = append(vals, string(val))
			}

			return nil
		})
	if err != nil {
		return nil, err
	}

	sort.Strings(vals)
	return connect.NewResponse(&ingestv1.LabelValuesResponse{
		Names: vals,
	}), nil
}

// ProfileTypes returns the possible profile types.
func (i *Ingester) ProfileTypes(ctx context.Context, req *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
	seen := map[string]struct{}{}
	err := i.engine.ScanTable("stacktraces").
		Distinct(
			logicalplan.Col(parcacol.ColumnName),
			logicalplan.Col(parcacol.ColumnSampleType),
			logicalplan.Col(parcacol.ColumnSampleUnit),
			logicalplan.Col(parcacol.ColumnPeriodType),
			logicalplan.Col(parcacol.ColumnPeriodUnit),
			logicalplan.Col(parcacol.ColumnDuration).GT(logicalplan.Literal(0)),
		).
		Execute(ctx, func(ar arrow.Record) error {
			if ar.NumCols() != 6 {
				return fmt.Errorf("expected 6 column, got %d", ar.NumCols())
			}

			nameColumn, err := binaryFieldFromRecord(ar, parcacol.ColumnName)
			if err != nil {
				return err
			}

			sampleTypeColumn, err := binaryFieldFromRecord(ar, parcacol.ColumnSampleType)
			if err != nil {
				return err
			}

			sampleUnitColumn, err := binaryFieldFromRecord(ar, parcacol.ColumnSampleUnit)
			if err != nil {
				return err
			}

			periodTypeColumn, err := binaryFieldFromRecord(ar, parcacol.ColumnPeriodType)
			if err != nil {
				return err
			}

			periodUnitColumn, err := binaryFieldFromRecord(ar, parcacol.ColumnPeriodUnit)
			if err != nil {
				return err
			}

			//

			for i := 0; i < int(ar.NumRows()); i++ {
				name := string(nameColumn.Value(i))
				sampleType := string(sampleTypeColumn.Value(i))
				sampleUnit := string(sampleUnitColumn.Value(i))
				periodType := string(periodTypeColumn.Value(i))
				periodUnit := string(periodUnitColumn.Value(i))

				key := fmt.Sprintf("%s:%s:%s:%s:%s", name, sampleType, sampleUnit, periodType, periodUnit)

				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}

			}

			return nil
		})
	if err != nil {
		return nil, err
	}

	types := make([]string, 0, len(seen))
	for key := range seen {
		types = append(types, key)
	}

	return connect.NewResponse(&ingestv1.ProfileTypesResponse{
		Names: types,
	}), nil
}

type selectMergeReq struct {
	query      string
	start, end int64
}

func (i *Ingester) selectMerge(ctx context.Context, query profileQuery, start, end int64) (*flamebearer.FlamebearerProfile, error) {
	filterExpr, err := mergePlan(query, start, end)
	if err != nil {
		// todo 4xx
		return nil, err
	}

	var ar arrow.Record
	err = i.engine.ScanTable("stacktraces").
		Filter(filterExpr).
		Aggregate(
			logicalplan.Sum(logicalplan.Col("value")),
			logicalplan.Col("stacktrace"),
		).
		Execute(ctx, func(r arrow.Record) error {
			r.Retain()
			ar = r
			return nil
		})
	if err != nil {
		return nil, err
	}
	defer ar.Release()
	flame, err := buildFlamebearer(ar, i.profileStore.MetaStore())
	if err != nil {
		return nil, err
	}
	unit := metadata.Units(query.sampleUnit)
	sampleRate := uint32(100)
	switch query.sampleType {
	case "inuse_objects", "alloc_objects", "goroutine", "samples":
		unit = metadata.ObjectsUnits
	case "cpu":
		unit = metadata.SamplesUnits
		sampleRate = uint32(100000000)

	}
	return &flamebearer.FlamebearerProfile{
		Version: 1,
		FlamebearerProfileV1: flamebearer.FlamebearerProfileV1{
			Flamebearer: *flame,
			Metadata: flamebearer.FlamebearerMetadataV1{
				Format:     "single",
				Units:      unit,
				Name:       query.sampleType,
				SampleRate: sampleRate,
			},
		},
	}, nil
}

func buildFlamebearer(ar arrow.Record, meta metastore.ProfileMetaStore) (*flamebearer.FlamebearerV1, error) {
	type sample struct {
		stacktraceID []byte
		locationIDs  [][]byte
		total        int64
		self         int64

		*metastore.Location
	}
	schema := ar.Schema()
	indices := schema.FieldIndices("stacktrace")
	if len(indices) != 1 {
		return nil, fmt.Errorf("expected exactly one stacktrace column, got %d", len(indices))
	}
	stacktraceColumn := ar.Column(indices[0]).(*array.Binary)

	indices = schema.FieldIndices("sum(value)")
	if len(indices) != 1 {
		return nil, fmt.Errorf("expected exactly one value column, got %d", len(indices))
	}
	valueColumn := ar.Column(indices[0]).(*array.Int64)

	rows := int(ar.NumRows())
	samples := make([]*sample, 0, rows)
	stacktraceUUIDs := make([][]byte, 0, rows)
	for i := 0; i < rows; i++ {
		stacktraceID := stacktraceColumn.Value(i)
		value := valueColumn.Value(i)
		stacktraceUUIDs = append(stacktraceUUIDs, stacktraceID)
		samples = append(samples, &sample{
			stacktraceID: stacktraceID,
			self:         value,
		})
	}

	stacktraceMap, err := meta.GetStacktraceByIDs(context.Background(), stacktraceUUIDs...)
	if err != nil {
		return nil, err
	}

	locationUUIDSeen := map[string]struct{}{}
	locationUUIDs := [][]byte{}
	for _, s := range stacktraceMap {
		for _, id := range s.GetLocationIds() {
			if _, seen := locationUUIDSeen[string(id)]; !seen {
				locationUUIDSeen[string(id)] = struct{}{}
				locationUUIDs = append(locationUUIDs, id)
			}
		}
	}

	locationMaps, err := metastore.GetLocationsByIDs(context.Background(), meta, locationUUIDs...)
	if err != nil {
		return nil, err
	}

	for _, s := range samples {
		s.locationIDs = stacktraceMap[string(s.stacktraceID)].LocationIds
	}

	stacks := make([]stack, 0, len(samples))
	for _, s := range samples {
		stack := stack{
			value: s.self,
		}

		for i := range s.locationIDs {
			stack.locations = append(stack.locations, location{
				function: locationMaps[string(s.locationIDs[i])].Lines[0].Function.Name,
			})
		}

		stacks = append(stacks, stack)
	}
	tree := stacksToTree(stacks)
	graph := tree.toFlamebearer()
	return graph, nil
}

// render/render?format=json&from=now-12h&until=now&query=pyroscope.server.cpu
func parseQueryRequest(req *http.Request) (selectMergeReq, error) {
	queryParams := req.URL.Query()
	q := queryParams.Get("query")
	if q == "" {
		return selectMergeReq{}, fmt.Errorf("query is required")
	}

	start := model.TimeFromUnixNano(time.Now().Add(-1 * time.Hour).UnixNano())
	end := model.TimeFromUnixNano(time.Now().UnixNano())

	if from := queryParams.Get("from"); from != "" {
		from, err := parseRelativeTime(from)
		if err != nil {
			return selectMergeReq{}, fmt.Errorf("failed to parse from: %w", err)
		}
		start = end.Add(-from)
	}

	return selectMergeReq{
		query: q,
		start: int64(start),
		end:   int64(end),
	}, nil
}

func mergePlan(query profileQuery, start, end int64) (logicalplan.Expr, error) {
	selectorExprs, err := queryToFilterExprs(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	return logicalplan.And(
		append(
			selectorExprs,
			logicalplan.Col("timestamp").GT(logicalplan.Literal(start)),
			logicalplan.Col("timestamp").LT(logicalplan.Literal(end)),
		)...,
	), nil
}

func parseRelativeTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "now-")
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return d, nil
}

type profileQuery struct {
	selector                                             []*labels.Matcher
	name, sampleType, sampleUnit, periodType, periodUnit string
	delta                                                bool
}

func parseQuery(q string) (profileQuery, error) {
	parsedSelector, err := parser.ParseMetricSelector(q)
	if err != nil {
		return profileQuery{}, status.Error(codes.InvalidArgument, "failed to parse query")
	}

	sel := make([]*labels.Matcher, 0, len(parsedSelector))
	var nameLabel *labels.Matcher
	for _, matcher := range parsedSelector {
		if matcher.Name == labels.MetricName {
			nameLabel = matcher
		} else {
			sel = append(sel, matcher)
		}
	}
	if nameLabel == nil {
		return profileQuery{}, status.Error(codes.InvalidArgument, "query must contain a profile-type selection")
	}

	parts := strings.Split(nameLabel.Value, ":")
	if len(parts) != 5 && len(parts) != 6 {
		return profileQuery{}, status.Errorf(codes.InvalidArgument, "profile-type selection must be of the form <name>:<sample-type>:<sample-unit>:<period-type>:<period-unit>(:delta), got(%d): %q", len(parts), nameLabel.Value)
	}
	name, sampleType, sampleUnit, periodType, periodUnit, delta := parts[0], parts[1], parts[2], parts[3], parts[4], false
	if len(parts) == 6 && parts[5] == "delta" {
		delta = true
	}
	return profileQuery{
		selector:   sel,
		name:       name,
		sampleType: sampleType,
		sampleUnit: sampleUnit,
		periodType: periodType,
		periodUnit: periodUnit,
		delta:      delta,
	}, nil
}

func queryToFilterExprs(q profileQuery) ([]logicalplan.Expr, error) {
	labelFilterExpressions, err := matchersToBooleanExpressions(q.selector)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to build query")
	}

	exprs := append([]logicalplan.Expr{
		logicalplan.Col("name").Eq(logicalplan.Literal(q.name)),
		logicalplan.Col("sample_type").Eq(logicalplan.Literal(q.sampleType)),
		logicalplan.Col("sample_unit").Eq(logicalplan.Literal(q.sampleUnit)),
		logicalplan.Col("period_type").Eq(logicalplan.Literal(q.periodType)),
		logicalplan.Col("period_unit").Eq(logicalplan.Literal(q.periodUnit)),
	}, labelFilterExpressions...)

	return exprs, nil
}

func matchersToBooleanExpressions(matchers []*labels.Matcher) ([]logicalplan.Expr, error) {
	exprs := make([]logicalplan.Expr, 0, len(matchers))

	for _, matcher := range matchers {
		expr, err := matcherToBooleanExpression(matcher)
		if err != nil {
			return nil, err
		}

		exprs = append(exprs, expr)
	}

	return exprs, nil
}

func binaryFieldFromRecord(ar arrow.Record, name string) (*array.Binary, error) {
	indices := ar.Schema().FieldIndices(name)
	if len(indices) != 1 {
		return nil, fmt.Errorf("expected 1 column named %q, got %d", name, len(indices))
	}

	col, ok := ar.Column(indices[0]).(*array.Binary)
	if !ok {
		return nil, fmt.Errorf("expected column %q to be a binary column, got %T", name, ar.Column(indices[0]))
	}

	return col, nil
}
