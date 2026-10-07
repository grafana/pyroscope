// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/util/strings.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package util

import "strings"

// StringsContain returns true if the search value is within the list of input values.
func StringsContain(values []string, search string) bool {
	for _, v := range values {
		if search == v {
			return true
		}
	}

	return false
}

// StringsMap returns a map where keys are input values.
func StringsMap(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, v := range values {
		out[v] = true
	}
	return out
}

// ToCamel converts s to CamelCase. Word boundaries are any non-letter,
// non-digit character (underscore, hyphen, space, etc.). Digits end a word, so
// the character that follows them is also capitalised.
func ToCamel(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	capNext := true
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
			capNext = false
		case r >= 'a' && r <= 'z':
			if capNext {
				b.WriteRune(r - ('a' - 'A'))
			} else {
				b.WriteRune(r)
			}
			capNext = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			capNext = true
		default:
			if b.Len() > 0 {
				capNext = true
			}
		}
	}
	return b.String()
}
