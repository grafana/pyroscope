package transporttrie

import (
	"bytes"
	"fmt"
	"hash"
	"hash/fnv"
	"math/rand"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/grafana/pyroscope/pkg/og/util/varint"
)

func randStr(l int) []byte {
	buf := make([]byte, l)
	for i := 0; i < l; i++ {
		buf[i] = byte(97) + byte(rand.Uint32()%10)
	}
	// rand.Read(buf)

	return buf
}

type trieHash struct {
	w varint.Writer
	h hash.Hash64
}

func newTrieHash() trieHash {
	return trieHash{
		w: varint.NewWriter(),
		h: fnv.New64a(),
	}
}

func (t *trieHash) addUint64(k []byte, v uint64) {
	_, _ = t.h.Write(k)
	_, _ = t.w.Write(t.h, v)
}

func (t *trieHash) addInt(k []byte, v int) {
	t.addUint64(k, uint64(v))
}

func (t trieHash) sum() uint64 {
	return t.h.Sum64()
}

var _ = Describe("trie package", func() {
	serializationExample := []byte("\x00\x00\x01\x02ab\x00\x02\x01c\x01\x00\x01d\x02\x00")
	Context("trie.Serialize()", func() {
		trie := New()
		trie.Insert([]byte("abc"), 1)
		trie.Insert([]byte("abd"), 2)
		logrus.Debug("trie abc abd", trie)

		It("returns correct results", func() {
			var buf bytes.Buffer
			trie.Serialize(&buf)
			Expect(buf.Bytes()).To(Equal(serializationExample))
		})

		Context("Ran 1000000 times", func() {
			var buf1 bytes.Buffer
			trie.Serialize(&buf1)
			It("returns the same result", func() {
				var buf2 bytes.Buffer
				trie.Serialize(&buf2)
				Expect(buf2).To(Equal(buf1))
			})
		})
	})

	Context("Ser/Deserialize()", func() {
		It("returns correct results", func() {
			for j := 0; j < 10; j++ {
				// logrus.Debug("---")
				trie := New()
				// trie.Insert([]byte("acc"), []byte{1})
				// trie.Insert([]byte("abc"), []byte{2})
				// trie.Insert([]byte("abd"), []byte{3})
				// trie.Insert([]byte("ab"), []byte{4})
				for i := 0; i < 10; i++ {
					trie.Insert(randStr(10), uint64(i))
				}
				// trie.Insert([]byte("abc"), []byte{1}, true)
				// trie.Insert([]byte("abc"), []byte{3}, true)
				// trie.Insert([]byte("bar"), []byte{5})
				// trie.Insert([]byte("abd"), []byte{2})
				// trie.Insert([]byte("abce"), []byte{3})
				// trie.Insert([]byte("ab"), []byte{4})
				// trie.Insert([]byte("abc"), []byte{2})

				// trie.Insert([]byte("baze"), []byte{1})
				// trie.Insert([]byte("baz"), []byte{2})
				// trie.Insert([]byte("bat"), []byte{3})
				// trie.Insert([]byte("bata"), []byte{4})
				// trie.Insert([]byte("batb"), []byte{5})
				// trie.Insert([]byte("bad"), []byte{6})
				// trie.Insert([]byte("bae"), []byte{7})
				// trie.Insert([]byte("zyx"), []byte{1})
				// trie.Insert([]byte("zy"), []byte{2})
				// trie.Insert([]byte(""), []byte{1})
				// trie.Insert([]byte("a"), []byte{2})
				// trie.Insert([]byte("b"), []byte{3})

				// trie.Insert([]byte("1234567"), []byte{1})
				// trie.Insert([]byte("1234667"), []byte{2})
				// trie.Insert([]byte("1234767"), []byte{3})
				logrus.Debug("a", trie.String())
				strA := ""
				trie.Iterate(func(k []byte, v uint64) {
					strA += fmt.Sprintf("%q %d\n", k, v)
				})
				logrus.Debug("strA", strA)

				var buf bytes.Buffer
				trie.Serialize(&buf)

				r := bytes.NewReader(buf.Bytes())
				t, e := Deserialize(r)
				strB := ""
				t.Iterate(func(k []byte, v uint64) {
					strB += fmt.Sprintf("%q %d\n", k, v)
				})
				logrus.Debug("b", t.String())
				logrus.Debug("strB", strB)
				Expect(e).To(BeNil())
				Expect(trie.String()).To(Equal(t.String()))
				Expect(strA).To(Equal(strB))
				logrus.Debug("---/")
			}
		})
	})

	Context("IterateRaw()", func() {
		compareWithRawIterator := func(t *Trie) {
			h1 := newTrieHash()
			t.Iterate(h1.addUint64)
			var buf bytes.Buffer
			Expect(t.Serialize(&buf)).ToNot(HaveOccurred())

			r := bytes.NewReader(buf.Bytes())
			h2 := newTrieHash()
			tmpBuf := make([]byte, 0, 256)
			Expect(IterateRaw(r, tmpBuf, h2.addInt)).ToNot(HaveOccurred())

			Expect(h2.sum()).To(Equal(h1.sum()))
		}

		It("returns correct results", func() {
			type value struct {
				k string
				v uint64
			}

			values := []value{
				{"foo;bar;baz", 1},
				{"foo;bar;baz;a", 1},
				{"foo;bar;baz;b", 1},
				{"foo;bar;baz;c", 1},
				{"foo;bar;bar", 1},
				{"foo;bar;qux", 1},
				{"foo;bax;bar", 1},
				{"zoo;boo", 1},
				{"zoo;bao", 1},
			}

			trie := New()
			for _, v := range values {
				trie.Insert([]byte(v.k), v.v)
			}

			compareWithRawIterator(trie)
		})

		It("handles random tries properly", func() {
			for j := 0; j < 10; j++ {
				trie := New()
				for i := 0; i < 10; i++ {
					trie.Insert(randStr(10), uint64(i))
				}

				h1 := newTrieHash()
				trie.Iterate(h1.addUint64)

				var buf bytes.Buffer
				err := trie.Serialize(&buf)
				Expect(err).To(BeNil())

				r := bytes.NewReader(buf.Bytes())
				h2 := newTrieHash()
				err = IterateRaw(r, nil, h2.addInt)
				Expect(err).To(BeNil())

				Expect(h2.sum()).To(Equal(h1.sum()))
			}
		})
	})

	Context("Deserialize()", func() {
		trie := New()
		trie.Insert([]byte("abc"), 1)
		trie.Insert([]byte("ab"), 2)
		logrus.Debug(trie.String())

		It("returns correct results", func() {
			r := bytes.NewReader(serializationExample)
			t, e := Deserialize(r)
			logrus.Debug(t.String())
			Expect(e).To(BeNil())
			var buf bytes.Buffer
			t.Serialize(&buf)
			Expect(buf.Bytes()).To(Equal(serializationExample))
		})

		Context("Ran 1000000 times", func() {
			var buf1 bytes.Buffer
			trie.Serialize(&buf1)
			It("returns the same result", func() {
				var buf2 bytes.Buffer
				trie.Serialize(&buf2)
				Expect(buf2).To(Equal(buf1))
			})
		})
	})
})
