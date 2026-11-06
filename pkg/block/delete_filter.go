package block

import (
	"fmt"
	"regexp"
)

// CompileServiceNameRegex compiles a dataset deletion filter as a full-string
// match. The expression is validated on its own first: wrapping an unbalanced
// expression such as `a)|(b` in anchors would otherwise yield a valid regex
// (`^(?:a)|(b)$`) with different, non-anchored semantics.
func CompileServiceNameRegex(expr string) (*regexp.Regexp, error) {
	if _, err := regexp.Compile(expr); err != nil {
		return nil, fmt.Errorf("invalid service_name_regex: %w", err)
	}
	re, err := regexp.Compile("^(?:" + expr + ")$")
	if err != nil {
		return nil, fmt.Errorf("invalid service_name_regex: %w", err)
	}
	return re, nil
}
