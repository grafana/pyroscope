package ingester

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"
	"github.com/bufbuild/connect-go"
	"github.com/gogo/status"
	"github.com/parca-dev/parca/pkg/metastore"
	"github.com/parca-dev/parca/pkg/parcacol"
	"github.com/polarsignals/arcticdb/query/logicalplan"
	"github.com/prometheus/prometheus/promql/parser"
	"google.golang.org/grpc/codes"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	ingesterv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/model"
	"github.com/grafana/fire/pkg/profilestore"
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
			fmt.Println(ar)
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

func (i *Ingester) SymbolizeStacktraces(ctx context.Context, req *connect.Request[ingestv1.SymbolizeStacktraceRequest]) (*connect.Response[ingestv1.SymbolizeStacktraceResponse], error) {
	stacktraceMap, err := i.profileStore.MetaStore().GetStacktraceByIDs(ctx, req.Msg.Ids...)
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

	locationMaps, err := metastore.GetLocationsByIDs(context.Background(), i.profileStore.MetaStore(), locationUUIDs...)
	if err != nil {
		return nil, err
	}
	uniqueFn := map[string]int{}
	var fns []string
	locations := make([]*ingestv1.Location, len(req.Msg.Ids))

	for i, s := range req.Msg.Ids {
		locIds := stacktraceMap[string(s)].LocationIds
		locs := &ingestv1.Location{
			Ids: make([]int32, len(locIds)),
		}
		for j, l := range locIds {
			fn := locationMaps[string(l)].Lines[0].Function.Name
			id, seen := uniqueFn[fn]
			if !seen {
				id = len(fns)
				fns = append(fns, fn)
				uniqueFn[fn] = id
			}
			locs.Ids[j] = int32(id)
		}
		locations[i] = locs
	}

	return connect.NewResponse(&ingesterv1.SymbolizeStacktraceResponse{
		Locations:     locations,
		FunctionNames: fns,
	}), nil
}

func (i *Ingester) SelectProfiles(ctx context.Context, req *connect.Request[ingestv1.SelectProfilesRequest]) (*connect.Response[ingestv1.SelectProfilesResponse], error) {
	selectors, err := parser.ParseMetricSelector(req.Msg.LabelSelector)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to label selector")
	}
	filterExpr, err := profilestore.FilterProfiles(profilestore.ProfileQuery{
		Name:       req.Msg.Type.Name,
		SampleType: req.Msg.Type.SampleType,
		PeriodType: req.Msg.Type.PeriodType,
		SampleUnit: req.Msg.Type.SampleUnit,
		PeriodUnit: req.Msg.Type.PeriodUnit,
		Selector:   selectors,
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
					model.LabelPairsString(labelSet),
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
					Labels:    model.CloneLabelPairs(labelSet),
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
	// sort by timestamp then labels.
	sort.Slice(result.Profiles, func(i, j int) bool {
		return model.CompareProfile(result.Profiles[i], result.Profiles[j]) < 0
	})
	return connect.NewResponse(result), nil
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
