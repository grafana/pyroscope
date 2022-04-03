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

func (s *Storage) GetKeysByQuery(_ context.Context, query string, cb func(_k string) bool) error {
	parsedQuery, err := flameql.ParseQuery(query)
	if err != nil {
		return err
	}

	segmentKey, err := segment.ParseKey(parsedQuery.AppName + "{}")
	if err != nil {
		return err
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

	resultList := []string{}
	for v := range resultSet {
		resultList = append(resultList, v)
	}

	sort.Strings(resultList)
	for _, v := range resultList {
		if !cb(v) {
			break
		}
	}
	return nil
}

func (s *Storage) GetValuesByQuery(_ context.Context, label string, query string, cb func(v string) bool) error {
	parsedQuery, err := flameql.ParseQuery(query)
	if err != nil {
		return err
	}

	segmentKey, err := segment.ParseKey(parsedQuery.AppName + "{}")
	if err != nil {
		return err
	}
	dimensionKeys := s.dimensionKeysByKey(segmentKey)

	resultSet := map[string]bool{}
	for _, dk := range dimensionKeys() {
		dkParsed, _ := segment.ParseKey(string(dk))
		if v, ok := dkParsed.Labels()[label]; ok {
			resultSet[v] = true
		}
	}

	resultList := []string{}
	for v := range resultSet {
		resultList = append(resultList, v)
	}

	sort.Strings(resultList)
	for _, v := range resultList {
		if !cb(v) {
			break
		}
	}
	return nil
}

// GetAppNames returns the list of all app's names
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
