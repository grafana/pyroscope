package storage

import (
	"context"
	"sort"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
)

//revive:disable-next-line:get-return callback is used
func (s *Storage) GetKeys(_ context.Context, cb func(string) bool) { s.labels.GetKeys(cb) }

//revive:disable-next-line:get-return callback is used
func (s *Storage) GetValues(_ context.Context, key string, cb func(v string) bool) {
	s.labels.GetValues(key, func(v string) bool {
		if key != "__name__" || !slices.StringContains(s.config.hideApplications, v) {
			return cb(v)
		}
		return true
	})
}

func (s *Storage) GetKeysByQuery(_ context.Context, in GetLabelKeysByQueryInput) (GetLabelKeysByQueryOutput, error) {
	var output GetLabelKeysByQueryOutput
	parsedQuery, err := flameql.ParseQuery(in.Query)
	if err != nil {
		return output, err
	}

	segmentKey, err := segment.ParseKey(parsedQuery.AppName + "{}")
	if err != nil {
		return output, err
	}
	dimensionKeys := s.dimensionKeysByKey(segmentKey)

	resultSet := map[string]bool{}
	for _, dk := range dimensionKeys() {
		dkParsed, _ := segment.ParseKey(string(dk))
		if dkParsed.AppName() == parsedQuery.AppName {
			for k := range dkParsed.Labels() {
				resultSet[k] = true
			}
		}
	}

	for v := range resultSet {
		output.Keys = append(output.Keys, v)
	}

	sort.Strings(output.Keys)
	return output, nil
}

func (s *Storage) GetValuesByQuery(_ context.Context, in GetLabelValuesByQueryInput) (GetLabelValuesByQueryOutput, error) {
	var output GetLabelValuesByQueryOutput
	parsedQuery, err := flameql.ParseQuery(in.Query)
	if err != nil {
		return output, err
	}

	segmentKey, err := segment.ParseKey(parsedQuery.AppName + "{}")
	if err != nil {
		return output, err
	}
	dimensionKeys := s.dimensionKeysByKey(segmentKey)

	resultSet := map[string]bool{}
	for _, dk := range dimensionKeys() {
		dkParsed, _ := segment.ParseKey(string(dk))
		if v, ok := dkParsed.Labels()[in.Label]; ok {
			resultSet[v] = true
		}
	}

	for v := range resultSet {
		output.Values = append(output.Values, v)
	}

	sort.Strings(output.Values)
	return output, nil
}

// GetAppNames returns the list of all app's names
// It works by querying the __name__ label
func (s *Storage) GetAppNames(ctx context.Context) []string {
	appNames := make([]string, 0)

	s.GetValues(ctx, "__name__", func(v string) bool {
		if strings.TrimSpace(v) != "" {
			// skip empty app names
			appNames = append(appNames, v)
		}

		return true
	})

	return appNames
}
