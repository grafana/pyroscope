package ingester

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"
	"github.com/bufbuild/connect-go"
	"github.com/gogo/status"
	"github.com/parca-dev/parca/pkg/metastore"
	"github.com/parca-dev/parca/pkg/parcacol"
	"github.com/polarsignals/arcticdb/query/logicalplan"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"google.golang.org/grpc/codes"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
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
	filterExpr, err := selectPlan(query, start, end)
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

func (i *Ingester) SelectProfiles(ctx context.Context, req *connect.Request[ingestv1.SelectProfilesRequest]) (*connect.Response[ingestv1.SelectProfilesResponse], error) {
	selectors, err := parser.ParseMetricSelector(req.Msg.LabelSelector)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to label selector")
	}
	filterExpr, err := selectPlan(profileQuery{
		name:       req.Msg.Type.Name,
		sampleType: req.Msg.Type.SampleType,
		periodType: req.Msg.Type.PeriodType,
		sampleUnit: req.Msg.Type.SampleUnit,
		periodUnit: req.Msg.Type.PeriodUnit,
		selector:   selectors,
	}, req.Msg.Start, req.Msg.End)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	staticColumns := []logicalplan.Expr{
		logicalplan.Col("name"),
		logicalplan.Col("sample_type"),
		logicalplan.Col("sample_unit"),
		logicalplan.Col("period_type"),
		logicalplan.Col("period_unit"),
		logicalplan.Col("stacktrace"),
		logicalplan.Col("value"),
		logicalplan.Col("timestamp"),
	}
	// find the label keys (dynamic columns) we need to select eg. labels.key1, labels.key2....
	var dynamicColums []logicalplan.Expr
	err = i.engine.ScanSchema("stacktraces").
		Distinct(logicalplan.Col("name")).
		Filter(logicalplan.Col("name").RegexMatch("^labels\\..+$")).
		Execute(ctx, func(r arrow.Record) error {
			r.Retain()
			col := r.Column(0).(*array.String)
			dynamicColums = append(dynamicColums, logicalplan.Col(col.Value(0)))
			return nil
		})
	if err != nil {
		return nil, err
	}

	colums := append(staticColumns, dynamicColums...)
	profileMap := make(map[string]*ingestv1.Profile)
	labelSet := []*commonv1.LabelPair{}
	err = i.engine.ScanTable("stacktraces").
		Project(colums...).
		Filter(filterExpr).
		Execute(ctx, func(ar arrow.Record) error {
			if ar.NumCols() < int64(len(staticColumns)) {
				return fmt.Errorf("expected %d columns, got %d", len(staticColumns), ar.NumCols())
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
			stacktraceColumn, err := binaryFieldFromRecord(ar, parcacol.ColumnStacktrace)
			if err != nil {
				return err
			}

			timestampColumn, err := int64FieldFromRecord(ar, parcacol.ColumnTimestamp)
			if err != nil {
				return err
			}
			valueColumn, err := int64FieldFromRecord(ar, parcacol.ColumnValue)
			if err != nil {
				return err
			}
			labelColumnIndices := []int{}
			fields := ar.Schema().Fields()
			for i, f := range fields {
				if strings.HasPrefix(f.Name, "labels.") {
					labelColumnIndices = append(labelColumnIndices, i)
				}
			}

			for i := 0; i < int(ar.NumRows()); i++ {
				labelSet = labelSet[:0]
				for _, j := range labelColumnIndices {
					col := ar.Column(j).(*array.Binary)
					if col.IsNull(i) {
						continue
					}

					v := col.Value(i)
					if len(v) > 0 {
						labelSet = append(labelSet, &commonv1.LabelPair{Name: strings.TrimPrefix(fields[j].Name, "labels."), Value: string(v)})
					}
				}
				sort.Slice(labelSet, func(i, j int) bool {
					return labelSet[i].Name < labelSet[j].Name
				})
				// todo(cyriltovena) we should use a buffer to avoid allocations
				profileKey := fmt.Sprintf("%s:%s:%s:%s:%s:%s:%d",
					labelPairString(labelSet),
					nameColumn.Value(i),
					sampleTypeColumn.Value(i),
					sampleUnitColumn.Value(i),
					periodTypeColumn.Value(i),
					periodUnitColumn.Value(i),
					timestampColumn.Value(i),
				)
				if profile, ok := profileMap[profileKey]; ok {
					profile.Stacktraces = append(profile.Stacktraces, &ingestv1.StacktraceSample{
						Value: valueColumn.Value(i),
						ID:    stacktraceColumn.Value(i),
					})
					continue
				}
				profile := &ingestv1.Profile{
					Type: &ingestv1.ProfileType{
						Name:       string(nameColumn.Value(i)),
						SampleType: string(sampleTypeColumn.Value(i)),
						SampleUnit: string(sampleUnitColumn.Value(i)),
						PeriodType: string(periodTypeColumn.Value(i)),
						PeriodUnit: string(periodUnitColumn.Value(i)),
					},
					Timestamp: timestampColumn.Value(i),
					Labels:    cloneLabelPairs(labelSet),
					Stacktraces: []*ingestv1.StacktraceSample{
						{
							Value: valueColumn.Value(i),
							ID:    stacktraceColumn.Value(i),
						},
					},
				}
				profileMap[profileKey] = profile
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	result := &ingestv1.SelectProfilesResponse{
		Profiles: make([]*ingestv1.Profile, 0, len(profileMap)),
	}
	for _, profile := range profileMap {
		result.Profiles = append(result.Profiles, profile)
	}
	return connect.NewResponse(result), nil
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

func labelPairString(lbs []*commonv1.LabelPair) string {
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

func cloneLabelPairs(lbs []*commonv1.LabelPair) []*commonv1.LabelPair {
	result := make([]*commonv1.LabelPair, len(lbs))
	for i, l := range lbs {
		result[i] = &commonv1.LabelPair{
			Name:  l.Name,
			Value: l.Value,
		}
	}
	return result
}

func selectPlan(query profileQuery, start, end int64) (logicalplan.Expr, error) {
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

func int64FieldFromRecord(ar arrow.Record, name string) (*array.Int64, error) {
	indices := ar.Schema().FieldIndices(name)
	if len(indices) != 1 {
		return nil, fmt.Errorf("expected 1 column named %q, got %d", name, len(indices))
	}

	col, ok := ar.Column(indices[0]).(*array.Int64)
	if !ok {
		return nil, fmt.Errorf("expected column %q to be a int64 column, got %T", name, ar.Column(indices[0]))
	}

	return col, nil
}
