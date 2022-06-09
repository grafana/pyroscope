package dynparquet

import (
	"errors"
	"sort"
	"strings"
)

var ErrMalformedDynamicColumns = errors.New("malformed dynamic columns string")

func serializeDynamicColumns(dynamicColumns map[string][]string) string {
	names := make([]string, 0, len(dynamicColumns))
	for name := range dynamicColumns {
		names = append(names, name)
	}
	sort.Strings(names)

	str := ""
	for i, name := range names {
		if i != 0 {
			str += ";"
		}
		str += name + ":" + strings.Join(dynamicColumns[name], ",")
	}

	return str
}

func deserializeDynamicColumns(dynColString string) (map[string][]string, error) {
	dynCols := map[string][]string{}

	// handle case where the schema has no dynamic columnns
	if len(dynColString) == 0 {
		return dynCols, nil
	}

	for _, dynString := range strings.Split(dynColString, ";") {
		split := strings.Split(dynString, ":")
		if len(split) != 2 {
			return nil, ErrMalformedDynamicColumns
		}
		labelValues := strings.Split(split[1], ",")
		if len(labelValues) == 1 && labelValues[0] == "" {
			labelValues = []string{}
		}
		dynCols[split[0]] = labelValues
	}

	return dynCols, nil
}
