package symdb

import (
	"fmt"
	"regexp"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// FrameMatcher tests whole stack traces against function names. It is
// scoped to one symbol partition, whose function and stack trace IDs are local.
type FrameMatcher struct {
	symbols      *Symbols
	includeExact map[uint32]int
	includeRegex map[uint32][]int
	exclude      map[uint32]struct{}
	seen         []bool
	locations    []uint64
	cache        map[uint32]bool
}

// NewFrameMatcher indexes exact and regex name matches in one symbol partition.
func NewFrameMatcher(symbols *Symbols, filter *typesv1.StackFrameFilter) (*FrameMatcher, error) {
	if filter == nil || len(filter.IncludeFunctionNames)+len(filter.ExcludeFunctionNames)+
		len(filter.IncludeFunctionNameRegexes)+len(filter.ExcludeFunctionNameRegexes) == 0 {
		return nil, nil
	}
	m := &FrameMatcher{
		symbols:      symbols,
		includeExact: make(map[uint32]int),
		includeRegex: make(map[uint32][]int),
		exclude:      make(map[uint32]struct{}),
		cache:        make(map[uint32]bool),
	}
	includeNames := make(map[string]int, len(filter.IncludeFunctionNames))
	for _, name := range filter.IncludeFunctionNames {
		if _, ok := includeNames[name]; !ok {
			includeNames[name] = len(includeNames)
		}
	}
	includeRegexes, err := compileFrameRegexes("include", filter.IncludeFunctionNameRegexes)
	if err != nil {
		return nil, err
	}
	excludeRegexes, err := compileFrameRegexes("exclude", filter.ExcludeFunctionNameRegexes)
	if err != nil {
		return nil, err
	}
	m.seen = make([]bool, len(includeNames)+len(includeRegexes))
	excludeNames := make(map[string]struct{}, len(filter.ExcludeFunctionNames))
	for _, name := range filter.ExcludeFunctionNames {
		excludeNames[name] = struct{}{}
	}
	for id, fn := range symbols.Functions {
		name := symbols.Strings[fn.Name]
		if i, found := includeNames[name]; found {
			m.includeExact[uint32(id)] = i
		}
		for i, re := range includeRegexes {
			if re.MatchString(name) {
				m.includeRegex[uint32(id)] = append(m.includeRegex[uint32(id)], len(includeNames)+i)
			}
		}
		_, excluded := excludeNames[name]
		if !excluded {
			for _, re := range excludeRegexes {
				if re.MatchString(name) {
					excluded = true
					break
				}
			}
		}
		if excluded {
			m.exclude[uint32(id)] = struct{}{}
		}
	}
	return m, nil
}

// ValidateFrameFilter rejects malformed regexes before a query reads data.
func ValidateFrameFilter(filter *typesv1.StackFrameFilter) error {
	if filter == nil {
		return nil
	}
	if _, err := compileFrameRegexes("include", filter.IncludeFunctionNameRegexes); err != nil {
		return err
	}
	_, err := compileFrameRegexes("exclude", filter.ExcludeFunctionNameRegexes)
	return err
}

func compileFrameRegexes(kind string, patterns []string) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid %s function name regex %q: %w", kind, pattern, err)
		}
		compiled = append(compiled, re)
	}
	return compiled, nil
}

// Matches reports whether a stack has every included name and no excluded name.
func (m *FrameMatcher) Matches(stacktraceID uint32) bool {
	if stacktraceID == 0 {
		return false
	}
	if matched, ok := m.cache[stacktraceID]; ok {
		return matched
	}
	for i := range m.seen {
		m.seen[i] = false
	}
	remaining := len(m.seen)
	m.locations = m.symbols.Stacktraces.LookupLocations(m.locations, stacktraceID)
	if len(m.locations) == 0 {
		m.cache[stacktraceID] = false
		return false
	}
	matched := true
	for _, locID := range m.locations {
		for _, line := range m.symbols.Locations[locID].Line {
			if _, excluded := m.exclude[line.FunctionId]; excluded {
				matched = false
				break
			}
			if i, included := m.includeExact[line.FunctionId]; included && !m.seen[i] {
				m.seen[i] = true
				remaining--
			}
			for _, i := range m.includeRegex[line.FunctionId] {
				if !m.seen[i] {
					m.seen[i] = true
					remaining--
				}
			}
		}
		if !matched {
			break
		}
	}
	matched = matched && remaining == 0
	m.cache[stacktraceID] = matched
	return matched
}
