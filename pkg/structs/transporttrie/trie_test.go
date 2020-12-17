package transporttrie

import (
	"bytes"
	"fmt"
	"math/rand"

	"github.com/pyroscope-io/pyroscope/pkg/structs/merge"
	log "github.com/sirupsen/logrus"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func randStr(l int) []byte {
	buf := make([]byte, l)
	for i := 0; i < l; i++ {
		buf[i] = byte(97) + byte(rand.Uint32()%10)
	}
	// rand.Read(buf)

	return buf
}

var _ = Describe("trie package", func() {
	serializationExample := []byte{0, 0, 1, 2, 97, 98, 0, 2, 1, 99, 1, 0, 1, 100, 2, 0}
	Context("trie.Serialize()", func() {
		trie := New()
		trie.Insert([]byte("abc"), 1)
		trie.Insert([]byte("abd"), 2)
		log.Debug("trie abc abd", trie)

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
				// log.Debug("---")
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
				log.Debug("a", trie.String())
				strA := ""
				trie.Iterate(func(k []byte, v uint64) {
					strA += fmt.Sprintf("%q %d\n", k, v)
				})
				log.Debug("strA", strA)

				var buf bytes.Buffer
				trie.Serialize(&buf)

				r := bytes.NewReader(buf.Bytes())
				t, e := Deserialize(r)
				strB := ""
				t.Iterate(func(k []byte, v uint64) {
					strB += fmt.Sprintf("%q %d\n", k, v)
				})
				log.Debug("b", t.String())
				log.Debug("strB", strB)
				Expect(e).To(BeNil())
				Expect(trie.String()).To(Equal(t.String()))
				Expect(strA).To(Equal(strB))
				log.Debug("---/")
			}
		})
	})

	Context("Deserialize()", func() {
		trie := New()
		trie.Insert([]byte("abc"), 1)
		trie.Insert([]byte("ab"), 2)
		log.Debug(trie.String())

		It("returns correct results", func() {
			r := bytes.NewReader(serializationExample)
			t, e := Deserialize(r)
			log.Debug(t.String())
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

	Context("MergeTriesConcurrently()", func() {
		It("merges 2 tries", func(done Done) {
			for s := 0; s < 1000; s++ {
				rand.Seed(int64(s))
				// log.Debug(s)
				t1 := New()
				t2 := New()
				t3 := New()
				// log.Debug("---")
				n := 2
				n2 := 4
				for i := 0; i < n; i++ {
					str := randStr(n2)
					t1.Insert(str, uint64(i))
					t3.Insert(str, uint64(i))
				}
				for i := 0; i < n; i++ {
					str := randStr(n2)
					t2.Insert(str, uint64(n+i))
					t3.Insert(str, uint64(n+i), true)
				}

				// t1 := New()
				// t1.Insert([]byte("abc"), []byte{1})
				// t1.Insert([]byte("abd"), []byte{2})
				// t1.Insert([]byte("abe"), []byte{2})

				// t2 := New()
				// t2.Insert([]byte("abc"), []byte{1})
				// t2.Insert([]byte("abd"), []byte{2})
				// t2.Insert([]byte("abf"), []byte{3})
				// t2.Insert([]byte("abef"), []byte{5})
				// t2.Insert([]byte("a"), []byte{6})
				// t2.Insert([]byte("ac"), []byte{7})
				// t2.Insert([]byte("aa"), []byte{8})

				// t3 := New()
				// t3.Insert([]byte("a"), []byte{6})
				// t3.Insert([]byte("ac"), []byte{7})
				// t3.Insert([]byte("aa"), []byte{8})
				// t3.Insert([]byte("abc"), []byte{2})
				// t3.Insert([]byte("abd"), []byte{4})
				// t3.Insert([]byte("abe"), []byte{2})
				// t3.Insert([]byte("abf"), []byte{3})
				// t3.Insert([]byte("abef"), []byte{5})

				var buf1 bytes.Buffer
				var buf2 bytes.Buffer
				t1.Serialize(&buf1)
				t2.Serialize(&buf2)

				// log.Debug("t1\n", t1.String())
				// log.Debug("t2\n", t2.String())
				// log.Debug("t3\n", t3.String())

				// Expect(buf1.Bytes()).To(Equal(buf2.Bytes()))
				tries := []merge.Merger{t1, t2}
				rand.Shuffle(len(tries), func(i, j int) {
					tries[i], tries[j] = tries[j], tries[i]
				})
				t1I := merge.MergeTriesSerially(1, tries...)
				t1 = t1I.(*Trie)
				// log.Debug("t1m\n", t1.String())

				var buf3 bytes.Buffer
				var buf4 bytes.Buffer
				t3.Serialize(&buf3)
				t1.Serialize(&buf4)
				Expect(buf4).To(Equal(buf3))
			}
			close(done)

		}, 1.0)
	})

	Context("trie.Merge()", func() {
		It("merges 2 tries", func() {
			t1 := New()
			t1.Insert([]byte("abc"), uint64(1))
			t1.Insert([]byte("abd"), uint64(2))

			t2 := New()
			t2.Insert([]byte("abc"), uint64(1))
			t2.Insert([]byte("abd"), uint64(2))

			t3 := New()
			t3.Insert([]byte("abc"), uint64(2))
			t3.Insert([]byte("abd"), uint64(4))

			var buf1 bytes.Buffer
			var buf2 bytes.Buffer
			t1.Serialize(&buf1)
			t2.Serialize(&buf2)

			log.Debug("t1", t1)
			log.Debug("t2", t2)
			log.Debug("t3", t3)

			Expect(buf1.Bytes()).To(Equal(buf2.Bytes()))
			t1.Merge(t2)

			log.Debug("t1+2", t1)

			var buf3 bytes.Buffer
			var buf4 bytes.Buffer
			t3.Serialize(&buf3)
			t1.Serialize(&buf4)
			Expect(buf4).To(Equal(buf3))
		})
	})
})
