package storage

import (
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	. "github.com/pyroscope-io/pyroscope/pkg/testing"
	"github.com/pyroscope-io/pyroscope/pkg/util/names"
)

func BenchmarkTags(b *testing.B) {
	RegisterFailHandler(Fail)
	logrus.SetOutput(io.Discard)

	withSuite(func(bs *tagSuite) {
		bs.k = newKeygen(newTag("__name__", 1))
		b.Run("no tags (0)", func(b *testing.B) {
			b.Run("put", bs.put)
			bs.fill(25000)
			b.Run("get", bs.get())
		})
	})

	withSuite(func(bs *tagSuite) {
		bs.k = newKeygen(
			newTag("__name__", 1),
			newTag("env", 4),
			newTag("region", 5),
			newTag("project", 25),
			newTag("version", 100),
		)
		b.Run("5 tags (12.5k)", func(b *testing.B) {
			b.Run("put", bs.put)
			bs.fill(25000)
			b.Run("get (app)", bs.get())
			b.Run("get (card 5)", bs.get("region"))
			b.Run("get (card 25)", bs.get("project"))
			b.Run("get (card 100)", bs.get("version"))
		})
	})

	withSuite(func(bs *tagSuite) {
		bs.k = newKeygen(
			newTag("__name__", 1),
			newTag("instance_id", 1000000),
			newTag("env", 4),
			newTag("region", 5),
			newTag("project", 25),
			newTag("version", 100),
		)
		b.Run("5 tags (inf)", func(b *testing.B) {
			b.Run("put", bs.put)
			bs.fill(25000)
			b.Run("get (app)", bs.get())
			b.Run("get (card 5)", bs.get("region"))
			b.Run("get (card 25)", bs.get("project"))
			b.Run("get (card 100)", bs.get("version"))
			b.Run("get (card 1000000)", bs.get("instance_id"))
		})
	})
}

type tagSuite struct {
	k *keygen
	s *Storage
	d *TmpDirectory
}

func withSuite(fn func(*tagSuite)) {
	bs := newSuite()
	defer bs.close()
	fn(bs)
}

func newSuite() *tagSuite {
	bs := tagSuite{
		d: TmpDirSync(),
	}
	x, err := New(&config.Server{
		StoragePath:           bs.d.Path,
		APIBindAddr:           ":4040",
		CacheEvictThreshold:   0.02,
		CacheEvictVolume:      0.10,
		MaxNodesSerialization: 2048,
		MaxNodesRender:        2048,
	})
	Expect(err).ToNot(HaveOccurred())
	bs.s = x
	return &bs
}

func (bs *tagSuite) close() {
	Expect(bs.s.Close()).ToNot(HaveOccurred())
	bs.d.Close()
}

func (bs *tagSuite) put(b *testing.B) {
	bs.fill(b.N)
}

func (bs *tagSuite) fill(n int) {
	for i := 0; i < n; i++ {
		bs.putTree()
	}
}

func (bs *tagSuite) putTree() {
	v := tree.New()
	v.Insert([]byte("a;b"), uint64(1))
	v.Insert([]byte("a;c"), uint64(2))
	st := time.Now().Add(time.Hour * 24 * 10 * -1)
	et := st.Add(time.Second * 10)
	err := bs.s.Put(&PutInput{
		StartTime:  st,
		EndTime:    et,
		Key:        bs.k.next(),
		Val:        v,
		SpyName:    "testspy",
		SampleRate: 100,
	})
	Expect(err).ToNot(HaveOccurred())
}

func (bs *tagSuite) get(tags ...string) func(b *testing.B) {
	bs.k.ixs = make([]int, len(bs.k.ixs))
	return func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			k := bs.k.next()
			q := map[string]string{"__name__": k.labels["__name__"]}
			for _, l := range tags {
				q[l] = k.labels[l]
			}
			k.labels = q
			_, err := bs.s.Get(&GetInput{
				StartTime: time.Now().Add(time.Hour * 24 * 11 * -1),
				EndTime:   time.Now(),
				Key:       k,
			})
			Expect(err).ToNot(HaveOccurred())
		}
	}
}

type keygen struct {
	tags []testTag
	ixs  []int
}

func newKeygen(tags ...testTag) *keygen {
	return &keygen{
		tags: tags,
		ixs:  make([]int, len(tags)),
	}
}

func (g *keygen) next() *Key {
	k := Key{labels: make(map[string]string)}
	for i := 0; i < len(g.tags); i++ {
		t := g.tags[i]
		k.labels[t.name] = t.values[g.ixs[i]%len(t.values)]
		g.ixs[i]++
	}
	return &k
}

type testTag struct {
	name   string
	values []string
}

func newTag(name string, c int) testTag {
	values := make([]string, c)
	for i := 0; i < c; i++ {
		values[i] = names.GetRandomName(uuid.New().String())
	}
	return testTag{name: name, values: values}
}
