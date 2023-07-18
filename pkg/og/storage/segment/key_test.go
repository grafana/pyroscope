package segment

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/grafana/pyroscope/pkg/og/flameql"
)

var _ = Describe("segment key", func() {
	Context("ParseKey", func() {
		It("no tags version works", func() {
			k, err := ParseKey("foo")
			Expect(err).ToNot(HaveOccurred())
			Expect(k.labels).To(Equal(map[string]string{"__name__": "foo"}))
		})

		It("simple values work", func() {
			k, err := ParseKey("foo{bar=1,baz=2}")
			Expect(err).ToNot(HaveOccurred())
			Expect(k.labels).To(Equal(map[string]string{"__name__": "foo", "bar": "1", "baz": "2"}))
		})

		It("simple values with spaces work", func() {
			k, err := ParseKey(" foo { bar = 1 , baz = 2 } ")
			Expect(err).ToNot(HaveOccurred())
			Expect(k.labels).To(Equal(map[string]string{"__name__": "foo", "bar": "1", "baz": "2"}))
		})
	})

	Context("Key", func() {
		Context("Normalize", func() {
			It("no tags version works", func() {
				k, err := ParseKey("foo")
				Expect(err).ToNot(HaveOccurred())
				Expect(k.Normalized()).To(Equal("foo{}"))
			})

			It("simple values work", func() {
				k, err := ParseKey("foo{bar=1,baz=2}")
				Expect(err).ToNot(HaveOccurred())
				Expect(k.Normalized()).To(Equal("foo{bar=1,baz=2}"))
			})

			It("unsorted values work", func() {
				k, err := ParseKey("foo{baz=1,bar=2}")
				Expect(err).ToNot(HaveOccurred())
				Expect(k.Normalized()).To(Equal("foo{bar=2,baz=1}"))
			})
		})
	})

	Context("Key", func() {
		Context("Match", func() {
			It("reports whether a segments key satisfies tag matchers", func() {
				type evalTestCase struct {
					query string
					match bool
					key   string
				}

				testCases := []evalTestCase{
					// No matchers specified except app name.
					{`app.name`, true, `app.name{foo=bar}`},
					{`app.name{}`, true, `app.name{foo=bar}`},
					{`app.name`, false, `x.name{foo=bar}`},
					{`app.name{}`, false, `x.name{foo=bar}`},

					{`app.name{foo="bar"}`, true, `app.name{foo=bar}`},
					{`app.name{foo!="bar"}`, true, `app.name{foo=baz}`},
					{`app.name{foo="bar"}`, false, `app.name{foo=baz}`},
					{`app.name{foo!="bar"}`, false, `app.name{foo=bar}`},

					// Tag key not present.
					{`app.name{foo="bar"}`, false, `app.name{bar=baz}`},
					{`app.name{foo!="bar"}`, true, `app.name{bar=baz}`},

					{`app.name{foo="bar",baz="qux"}`, true, `app.name{foo=bar,baz=qux}`},
					{`app.name{foo="bar",baz!="qux"}`, true, `app.name{foo=bar,baz=fred}`},
					{`app.name{foo="bar",baz="qux"}`, false, `app.name{foo=bar}`},
					{`app.name{foo="bar",baz!="qux"}`, false, `app.name{foo=bar,baz=qux}`},
					{`app.name{foo="bar",baz!="qux"}`, false, `app.name{baz=fred,bar=baz}`},
				}

				for _, tc := range testCases {
					qry, _ := flameql.ParseQuery(tc.query)
					k, _ := ParseKey(tc.key)
					if matched := k.Match(qry); matched != tc.match {
						Expect(matched).To(Equal(tc.match), tc.query, tc.key)
					}
				}
			})
		})
	})
})
