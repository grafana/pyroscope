package transporttrie

import (
	"bytes"
	"fmt"
	"hash"
	"hash/fnv"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/og/util/varint"
)

func randStr(l int) []byte {
	buf := make([]byte, l)
	for i := 0; i < l; i++ {
		buf[i] = byte(97) + byte(rand.Uint32()%10)
	}
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

var serializationExample = []byte("\x00\x00\x01\x02ab\x00\x02\x01c\x01\x00\x01d\x02\x00")

func TestTrieSerialize(t *testing.T) {
	trie := New()
	trie.Insert([]byte("abc"), 1)
	trie.Insert([]byte("abd"), 2)

	t.Run("returns correct results", func(t *testing.T) {
		var buf bytes.Buffer
		trie.Serialize(&buf)
		require.Equal(t, serializationExample, buf.Bytes())
	})

	t.Run("returns the same result on repeated serialize", func(t *testing.T) {
		var buf1 bytes.Buffer
		trie.Serialize(&buf1)

		var buf2 bytes.Buffer
		trie.Serialize(&buf2)
		require.Equal(t, buf1, buf2)
	})
}

func TestSerDeserialize(t *testing.T) {
	t.Run("returns correct results", func(t *testing.T) {
		for j := 0; j < 10; j++ {
			trie := New()
			for i := 0; i < 10; i++ {
				trie.Insert(randStr(10), uint64(i))
			}

			strA := ""
			trie.Iterate(func(k []byte, v uint64) {
				strA += fmt.Sprintf("%q %d\n", k, v)
			})

			var buf bytes.Buffer
			trie.Serialize(&buf)

			r := bytes.NewReader(buf.Bytes())
			deserialized, err := Deserialize(r)
			require.NoError(t, err)

			strB := ""
			deserialized.Iterate(func(k []byte, v uint64) {
				strB += fmt.Sprintf("%q %d\n", k, v)
			})
			require.Equal(t, trie.String(), deserialized.String())
			require.Equal(t, strA, strB)
		}
	})
}

func TestIterateRaw(t *testing.T) {
	compareWithRawIterator := func(t *testing.T, tr *Trie) {
		t.Helper()

		h1 := newTrieHash()
		tr.Iterate(h1.addUint64)
		var buf bytes.Buffer
		require.NoError(t, tr.Serialize(&buf))

		r := bytes.NewReader(buf.Bytes())
		h2 := newTrieHash()
		tmpBuf := make([]byte, 0, 256)
		require.NoError(t, IterateRaw(r, tmpBuf, h2.addInt))

		require.Equal(t, h1.sum(), h2.sum())
	}

	t.Run("returns correct results", func(t *testing.T) {
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

		compareWithRawIterator(t, trie)
	})

	t.Run("handles random tries properly", func(t *testing.T) {
		for j := 0; j < 10; j++ {
			trie := New()
			for i := 0; i < 10; i++ {
				trie.Insert(randStr(10), uint64(i))
			}

			h1 := newTrieHash()
			trie.Iterate(h1.addUint64)

			var buf bytes.Buffer
			err := trie.Serialize(&buf)
			require.NoError(t, err)

			r := bytes.NewReader(buf.Bytes())
			h2 := newTrieHash()
			err = IterateRaw(r, nil, h2.addInt)
			require.NoError(t, err)

			require.Equal(t, h1.sum(), h2.sum())
		}
	})
}

func TestDeserialize(t *testing.T) {
	trie := New()
	trie.Insert([]byte("abc"), 1)
	trie.Insert([]byte("ab"), 2)

	t.Run("returns correct results", func(t *testing.T) {
		r := bytes.NewReader(serializationExample)
		deserialized, err := Deserialize(r)
		require.NoError(t, err)

		var buf bytes.Buffer
		deserialized.Serialize(&buf)
		require.Equal(t, serializationExample, buf.Bytes())
	})

	t.Run("returns the same result on repeated serialize", func(t *testing.T) {
		var buf1 bytes.Buffer
		trie.Serialize(&buf1)

		var buf2 bytes.Buffer
		trie.Serialize(&buf2)
		require.Equal(t, buf1, buf2)
	})
}
