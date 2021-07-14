package storage

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/pyroscope-io/pyroscope/pkg/testing"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

func TestX(t *testing.T) {
	RegisterTestingT(t)
	RegisterFailHandler(Fail)
	logrus.SetLevel(logrus.InfoLevel)

	tmpDir := TmpDirSync()
	defer tmpDir.Close()
	cfg := &config.Config{
		Server: config.Server{
			StoragePath:           tmpDir.Path,
			APIBindAddr:           ":4040",
			CacheEvictThreshold:   0.02,
			CacheEvictVolume:      0.10,
			MaxNodesSerialization: 2048,
			MaxNodesRender:        2048,
		},
	}

	var err error
	s, err = New(&cfg.Server)
	Expect(err).ToNot(HaveOccurred())
	keys := []string{
		"app.name{foo=bar,baz=quo}",
		"app.name{foo=bar,baz=xxx}",
	}

	for _, k := range keys {
		put(k)
	}

	type testCase struct {
		query       string
		segmentKeys []dimension.Key
	}

	testCases := []testCase{
		{`app.name`, []dimension.Key{
			dimension.Key("app.name{baz=quo,foo=bar}"),
			dimension.Key("app.name{baz=xxx,foo=bar}"),
		}},
		{`app.name{foo="bar"}`, []dimension.Key{
			dimension.Key("app.name{baz=quo,foo=bar}"),
			dimension.Key("app.name{baz=xxx,foo=bar}"),
		}},
		{`app.name{foo=~"b.*"}`, []dimension.Key{
			dimension.Key("app.name{baz=quo,foo=bar}"),
			dimension.Key("app.name{baz=xxx,foo=bar}"),
		}},
		{`app.name{baz=~"xxx|quo"}`, []dimension.Key{
			dimension.Key("app.name{baz=quo,foo=bar}"),
			dimension.Key("app.name{baz=xxx,foo=bar}"),
		}},
		{`app.name{baz!="xxx"}`, []dimension.Key{
			dimension.Key("app.name{baz=quo,foo=bar}"),
		}},
		{`app.name{baz!~"x.*"}`, []dimension.Key{
			dimension.Key("app.name{baz=quo,foo=bar}"),
		}},
		{`app.name{foo!="bar"}`, nil},
	}

	for _, tc := range testCases {
		r, err := s.Query(context.TODO(), tc.query)
		Expect(err).ToNot(HaveOccurred())
		if tc.segmentKeys == nil {
			Expect(r).To(BeEmpty())
		} else {
			Expect(r).To(Equal(tc.segmentKeys))
		}
	}
}

func put(k string) {
	t := tree.New()
	t.Insert([]byte("a;b"), uint64(1))
	t.Insert([]byte("a;c"), uint64(2))
	st := SimpleTime(10)
	et := SimpleTime(19)
	key, _ := ParseKey(k)
	Expect(s.Put(&PutInput{
		StartTime:  st,
		EndTime:    et,
		Key:        key,
		Val:        t,
		SpyName:    "testspy",
		SampleRate: 100,
	})).ToNot(HaveOccurred())
}
