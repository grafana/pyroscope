package pyroql

import (
	"errors"
	"regexp/syntax"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestParseQuery(t *testing.T) {
	RegisterFailHandler(Fail)
	RegisterTestingT(t)

	type testCase struct {
		query string
		err   error
		q     *Query
	}

	testCases := []testCase{
		{`app.name`, nil, &Query{AppName: "app.name"}},
		{`app.name{}`, nil, &Query{AppName: "app.name"}},
		{`app.name{foo="bar"}`, nil,
			&Query{"app.name", []*TagMatcher{{"foo", "bar", EQL, nil}}}},
		{`app.name{foo="bar",baz!="quo"}`, nil,
			&Query{"app.name", []*TagMatcher{{"foo", "bar", EQL, nil}, {"baz", "quo", NEQ, nil}}}},

		{"", ErrAppNameIsRequired, nil},
		{"{}", ErrAppNameIsRequired, nil},
		{`app.name{,}`, ErrInvalidMatchersSyntax, nil},
		{`app.name[foo="bar"]`, ErrInvalidAppName, nil},
		{`app=name{}"`, ErrInvalidAppName, nil},
		{`app.name{foo="bar"`, ErrInvalidQuerySyntax, nil},
		{`app.name{__name__="foo"}`, ErrKeyReserved, nil},
	}

	for _, tc := range testCases {
		t.Log("query:", tc.query)
		q, err := ParseQuery(tc.query)
		if tc.err != nil {
			t.Logf("\texpected error: %v; actual: %v\n", tc.err, err)
			Expect(errors.Is(err, tc.err)).To(BeTrue())
		} else {
			Expect(err).To(BeNil())
		}
		Expect(q).To(Equal(tc.q))
	}
}

func TestParseMatcher(t *testing.T) {
	RegisterFailHandler(Fail)
	RegisterTestingT(t)

	type testCase struct {
		expr string
		err  error
		m    *TagMatcher
	}

	testCases := []testCase{
		{expr: `foo="bar"`, m: &TagMatcher{"foo", "bar", EQL, nil}},
		{expr: `foo="z"`, m: &TagMatcher{"foo", "z", EQL, nil}},
		{expr: `foo=""`, err: ErrInvalidValueSyntax},
		{expr: `foo="`, err: ErrInvalidValueSyntax},
		{expr: `foo="z`, err: ErrInvalidValueSyntax},
		{expr: `foo=`, err: ErrInvalidValueSyntax},

		{expr: `foo!="bar"`, m: &TagMatcher{"foo", "bar", NEQ, nil}},
		{expr: `foo!="z"`, m: &TagMatcher{"foo", "z", NEQ, nil}},
		{expr: `foo=~""`, err: ErrInvalidValueSyntax},
		{expr: `foo=~"`, err: ErrInvalidValueSyntax},
		{expr: `foo=~"z`, err: ErrInvalidValueSyntax},
		{expr: `foo=~`, err: ErrInvalidValueSyntax},

		{expr: `foo=~"bar"`, m: &TagMatcher{"foo", "bar", EQL_REGEX, nil}},
		{expr: `foo=~"z"`, m: &TagMatcher{"foo", "z", EQL_REGEX, nil}},
		{expr: `foo!=""`, err: ErrInvalidValueSyntax},
		{expr: `foo!="`, err: ErrInvalidValueSyntax},
		{expr: `foo!="z`, err: ErrInvalidValueSyntax},
		{expr: `foo!=`, err: ErrInvalidValueSyntax},

		{expr: `foo!~"bar"`, m: &TagMatcher{"foo", "bar", NEQ_REGEX, nil}},
		{expr: `foo!~"z"`, m: &TagMatcher{"foo", "z", NEQ_REGEX, nil}},
		{expr: `foo!~""`, err: ErrInvalidValueSyntax},
		{expr: `foo!~"`, err: ErrInvalidValueSyntax},
		{expr: `foo!~"z`, err: ErrInvalidValueSyntax},
		{expr: `foo!~`, err: ErrInvalidValueSyntax},

		{expr: `foo;bar="baz"`, err: ErrInvalidKey},
		{expr: `foo""`, err: ErrInvalidKey},
		{expr: `foo`, err: ErrMatchOperatorIsRequired},
		{expr: `foo!`, err: ErrInvalidValueSyntax},
		{expr: `foo!!`, err: ErrInvalidValueSyntax},
		{expr: `foo!~@b@"`, err: ErrInvalidValueSyntax},
		{expr: `foo!"bar"`, err: ErrUnknownOp},
		{expr: `foo!!"bar"`, err: ErrUnknownOp},
		{expr: `foo=="bar"`, err: ErrUnknownOp},
	}

	for _, tc := range testCases {
		t.Log("expr:", tc.expr)
		tm, err := ParseMatcher(tc.expr)
		if tc.err != nil {
			t.Logf("\texpected error: %v; actual: %v\n", tc.err, err)
			Expect(errors.Is(err, tc.err)).To(BeTrue())
		} else {
			Expect(err).To(BeNil())
		}
		if tm != nil {
			tm.R = nil
		}
		Expect(tm).To(Equal(tc.m))
	}
}

func TestParseMatcher_Regex(t *testing.T) {
	RegisterFailHandler(Fail)
	RegisterTestingT(t)

	t.Run("Valid", func(t *testing.T) {
		m, err := ParseMatcher(`foo=~".*_suffix"`)
		Expect(err).To(BeNil())
		Expect(m).ToNot(BeNil())
	})

	t.Run("Invalid", func(t *testing.T) {
		m, err := ParseMatcher(`foo=~"["`)
		Expect(m).To(BeNil())
		Expect(err).ToNot(BeNil())

		var e1 *syntax.Error
		Expect(errors.As(err, &e1)).To(BeTrue())

		var e2 *Error
		Expect(errors.As(err, &e2)).To(BeTrue())
	})
}
