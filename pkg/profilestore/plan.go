package profilestore

import (
	"fmt"

	"github.com/gogo/status"
	"github.com/polarsignals/arcticdb/query/logicalplan"
	"github.com/prometheus/prometheus/model/labels"
	"google.golang.org/grpc/codes"
)

type ProfileQuery struct {
	Selector                                             []*labels.Matcher
	Name, SampleType, SampleUnit, PeriodType, PeriodUnit string
	Delta                                                bool
}

func FilterProfiles(query ProfileQuery, start, end int64) (logicalplan.Expr, error) {
	selectorExprs, err := queryToFilterExprs(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	return logicalplan.And(
		append(
			selectorExprs,
			logicalplan.Col("timestamp").GT(logicalplan.Literal(start)),
			logicalplan.Col("timestamp").LT(logicalplan.Literal(end)),
		)...,
	), nil
}

func queryToFilterExprs(q ProfileQuery) ([]logicalplan.Expr, error) {
	labelFilterExpressions, err := matchersToBooleanExpressions(q.Selector)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to build query")
	}

	exprs := append([]logicalplan.Expr{
		logicalplan.Col("name").Eq(logicalplan.Literal(q.Name)),
		logicalplan.Col("sample_type").Eq(logicalplan.Literal(q.SampleType)),
		logicalplan.Col("sample_unit").Eq(logicalplan.Literal(q.SampleUnit)),
		logicalplan.Col("period_type").Eq(logicalplan.Literal(q.PeriodType)),
		logicalplan.Col("period_unit").Eq(logicalplan.Literal(q.PeriodUnit)),
	}, labelFilterExpressions...)

	return exprs, nil
}

func matchersToBooleanExpressions(matchers []*labels.Matcher) ([]logicalplan.Expr, error) {
	exprs := make([]logicalplan.Expr, 0, len(matchers))

	for _, matcher := range matchers {
		expr, err := matcherToBooleanExpression(matcher)
		if err != nil {
			return nil, err
		}

		exprs = append(exprs, expr)
	}

	return exprs, nil
}

func matcherToBooleanExpression(matcher *labels.Matcher) (logicalplan.Expr, error) {
	ref := logicalplan.Col("labels." + matcher.Name)
	switch matcher.Type {
	case labels.MatchEqual:
		return ref.Eq(logicalplan.Literal(matcher.Value)), nil
	case labels.MatchNotEqual:
		return ref.NotEq(logicalplan.Literal(matcher.Value)), nil
	case labels.MatchRegexp:
		return ref.RegexMatch(matcher.Value), nil
	case labels.MatchNotRegexp:
		return ref.RegexNotMatch(matcher.Value), nil
	default:
		return nil, fmt.Errorf("unsupported matcher type %v", matcher.Type.String())
	}
}
