package flameql

import "regexp"

type Query struct {
	AppName  string
	Matchers []*TagMatcher

	q string // The original query string.
}

func (q *Query) String() string { return q.q }

type TagMatcher struct {
	Key   string
	Value string
	Op

	R *regexp.Regexp
}

type Op int

const (
	// The order should respect operator priority and cost.
	// Negating operators go first. See IsNegation.
	_         Op = iota
	NEQ          // !=
	NEQ_REGEX    // !~
	EQL          // =
	EQL_REGEX    // =~
)

// IsNegation reports whether the operator assumes negation.
func (o Op) IsNegation() bool { return o < EQL }

// ByPriority is a supplemental type for sorting tag matchers.
type ByPriority []*TagMatcher

func (p ByPriority) Len() int           { return len(p) }
func (p ByPriority) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p ByPriority) Less(i, j int) bool { return p[i].Op < p[j].Op }

func (m *TagMatcher) Match(v string) bool {
	switch m.Op {
	case EQL:
		return m.Value == v
	case NEQ:
		return m.Value != v
	case EQL_REGEX:
		return m.R.Match([]byte(v))
	case NEQ_REGEX:
		return !m.R.Match([]byte(v))
	default:
		panic("invalid match operator")
	}
}
