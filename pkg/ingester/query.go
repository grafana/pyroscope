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
	metastorev1alpha1 "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
	"github.com/parca-dev/parca/pkg/parcacol"
	"github.com/parca-dev/parca/pkg/profile"
	"github.com/polarsignals/frostdb/query/logicalplan"
	"github.com/prometheus/prometheus/promql/parser"
	"google.golang.org/grpc/codes"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	ingesterv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/model"
	"github.com/grafana/fire/pkg/profilestore"
	"github.com/grafana/fire/pkg/util"
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
		).
		Execute(ctx, func(ar arrow.Record) error {
			if ar.NumCols() != 5 {
				return fmt.Errorf("expected 5 column, got %d", ar.NumCols())
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
	// return stacktraceLocations, nil
	stracktracesIDs := make([]string, 0, len(req.Msg.Ids))
	for _, id := range req.Msg.Ids {
		id := id
		stracktracesIDs = append(stracktracesIDs, util.UnsafeGetString(id))
	}
	sres, err := i.profileStore.Metastore().Stacktraces(ctx, &metastorev1alpha1.StacktracesRequest{StacktraceIds: stracktracesIDs})
	if err != nil {
		return nil, err
	}
	locationNum := 0
	for _, stacktrace := range sres.Stacktraces {
		locationNum += len(stacktrace.LocationIds)
	}

	locationIndex := make(map[string]int, locationNum)
	locationIDs := make([]string, 0, locationNum)
	for _, s := range sres.Stacktraces {
		for _, id := range s.LocationIds {
			if _, seen := locationIndex[id]; !seen {
				locationIDs = append(locationIDs, id)
				locationIndex[id] = len(locationIDs) - 1
			}
		}
	}

	lres, err := i.profileStore.Metastore().Locations(ctx, &metastorev1alpha1.LocationsRequest{LocationIds: locationIDs})
	if err != nil {
		return nil, err
	}

	locations, err := getLocationsFromSerializedLocations(ctx, i.profileStore.Metastore(), locationIDs, lres.Locations)
	if err != nil {
		return nil, err
	}

	uniqueFn := map[string]int{}
	var fns []string
	locationResults := make([]*ingestv1.Location, len(req.Msg.Ids))

	for i, stacktrace := range sres.Stacktraces {
		locs := &ingestv1.Location{
			Ids: make([]int32, len(stacktrace.LocationIds)),
		}
		for j, id := range stacktrace.LocationIds {
			fn := locations[locationIndex[id]].Lines[0].Function.Name
			id, seen := uniqueFn[fn]
			if !seen {
				id = len(fns)
				fns = append(fns, fn)
				uniqueFn[fn] = id
			}
			locs.Ids[j] = int32(id)
		}
		locationResults[i] = locs
	}

	return connect.NewResponse(&ingesterv1.SymbolizeStacktraceResponse{
		Locations:     locationResults,
		FunctionNames: fns,
	}), nil
}

func getLocationsFromSerializedLocations(
	ctx context.Context,
	s metastorev1alpha1.MetastoreServiceClient,
	locationIds []string,
	locations []*metastorev1alpha1.Location,
) (
	[]*profile.Location,
	error,
) {
	mappingIndex := map[string]int{}
	mappingIDs := []string{}
	for _, location := range locations {
		if location.MappingId == "" {
			continue
		}

		if _, found := mappingIndex[location.MappingId]; !found {
			mappingIDs = append(mappingIDs, location.MappingId)
			mappingIndex[location.MappingId] = len(mappingIDs) - 1
		}
	}

	var mappings []*metastorev1alpha1.Mapping
	if len(mappingIDs) > 0 {
		mres, err := s.Mappings(ctx, &metastorev1alpha1.MappingsRequest{
			MappingIds: mappingIDs,
		})
		if err != nil {
			return nil, fmt.Errorf("get mappings by IDs: %w", err)
		}
		mappings = mres.Mappings
	}

	lres, err := s.LocationLines(ctx, &metastorev1alpha1.LocationLinesRequest{
		LocationIds: locationIds,
	})
	if err != nil {
		return nil, fmt.Errorf("get lines by location IDs: %w", err)
	}

	functionIndex := map[string]int{}
	functionIDs := []string{}
	for _, lines := range lres.LocationLines {
		for _, line := range lines.Entries {
			if _, found := functionIndex[line.FunctionId]; !found {
				functionIDs = append(functionIDs, line.FunctionId)
				functionIndex[line.FunctionId] = len(functionIDs) - 1
			}
		}
	}

	fres, err := s.Functions(ctx, &metastorev1alpha1.FunctionsRequest{
		FunctionIds: functionIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("get functions by ids: %w", err)
	}

	res := make([]*profile.Location, 0, len(locations))
	for i, location := range locations {
		var mapping *metastorev1alpha1.Mapping
		if location.MappingId != "" {
			mapping = mappings[mappingIndex[location.MappingId]]
		}

		lines := lres.LocationLines[i].Entries
		symbolizedLines := make([]profile.LocationLine, 0, len(lines))
		for _, line := range lines {
			symbolizedLines = append(symbolizedLines, profile.LocationLine{
				Function: fres.Functions[functionIndex[line.FunctionId]],
				Line:     line.Line,
			})
		}

		res = append(res, &profile.Location{
			ID:       location.Id,
			Address:  location.Address,
			IsFolded: location.IsFolded,
			Mapping:  mapping,
			Lines:    symbolizedLines,
		})
	}

	return res, nil
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
			col := r.Column(0).(*array.String)
			for i := 0; i < col.Len(); i++ {
				dynamicColums = append(dynamicColums, logicalplan.Col(col.Value(i)))
			}
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
