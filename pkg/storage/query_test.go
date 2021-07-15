package storage

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/pyroql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("querying", func() {
	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			var err error
			s, err = New(&(*cfg).Server)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("basic queries", func() {
			It("returns error on invalid query", func() {
				_, err := s.Query(context.TODO(), `app.name{foo=bar}`)
				Expect(errors.Is(err, pyroql.ErrInvalidValueSyntax)).To(BeTrue())
			})

			It("return expected results", func() {
				keys := []string{
					"app.name{foo=bar,baz=qux}",
					"app.name{foo=bar,baz=xxx}",
					"app.name{waldo=fred,baz=xxx}",
				}
				for _, k := range keys {
					put(s, k)
				}

				type testCase struct {
					query       string
					segmentKeys []dimension.Key
				}

				testCases := []testCase{
					{`app.name`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{foo="bar"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
					}},
					{`app.name{foo=~"^b.*"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
					}},
					{`app.name{baz=~"xxx|qux"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{baz!="xxx"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
					}},
					{`app.name{foo!="bar"}`, []dimension.Key{
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{foo!~".*"}`, []dimension.Key{
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{baz!~"^x.*"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
					}},
					{`app.name{foo="bar",baz!~"^x.*"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
					}},
					{`app.name{foo=~"b.*",foo!~".*r"}`, nil},
					{`not.an.app{}`, nil},
				}

				for _, tc := range testCases {
					r, err := s.Query(context.TODO(), tc.query)
					Expect(err).ToNot(HaveOccurred())
					if tc.segmentKeys == nil {
						Expect(r).To(BeEmpty())
						continue
					}
					Expect(r).To(Equal(tc.segmentKeys))
				}
			})
		})
	})
})

func put(s *Storage, k string) {
	t := tree.New()
	t.Insert([]byte("a;b"), uint64(1))
	t.Insert([]byte("a;c"), uint64(2))
	st := testing.SimpleTime(10)
	et := testing.SimpleTime(19)
	key, err := ParseKey(k)
	Expect(err).ToNot(HaveOccurred())
	err = s.Put(&PutInput{
		StartTime:  st,
		EndTime:    et,
		Key:        key,
		Val:        t,
		SpyName:    "testspy",
		SampleRate: 100,
	})
	Expect(err).ToNot(HaveOccurred())
}
