package ingester

import (
	"context"
	"fmt"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"
	"github.com/bufbuild/connect-go"

	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
)

// LabelValues returns the possible label values for a given label name.
func (i *Ingester) LabelValues(ctx context.Context, req *connect.Request[ingestv1.LabelValuesRequest]) (*connect.Response[ingestv1.LabelValuesResponse], error) {
	return i.fireDB.Head().LabelValues(ctx, req)
}

// ProfileTypes returns the possible profile types.
func (i *Ingester) ProfileTypes(ctx context.Context, req *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
	return i.fireDB.Head().ProfileTypes(ctx, req)
}

// Series returns labels series for the given set of matchers.
func (i *Ingester) Series(ctx context.Context, req *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error) {
	return i.fireDB.Head().Series(ctx, req)
}

/*
func (i *Ingester) SymbolizeStacktraces(ctx context.Context, req *connect.Request[ingestv1.SymbolizeStacktraceRequest]) (*connect.Response[ingestv1.SymbolizeStacktraceResponse], error) {
	return nil, errors.New("not implemented")

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
*/
/*
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
*/

func (i *Ingester) SelectProfiles(ctx context.Context, req *connect.Request[ingestv1.SelectProfilesRequest]) (*connect.Response[ingestv1.SelectProfilesResponse], error) {
	return i.fireDB.SelectProfiles(ctx, req)
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
