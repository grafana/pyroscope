package transporttrie

import (
	"bytes"

	"github.com/sirupsen/logrus"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("trie package", func() {
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

			logrus.Debug("t1", t1)
			logrus.Debug("t2", t2)
			logrus.Debug("t3", t3)

			Expect(buf1.Bytes()).To(Equal(buf2.Bytes()))
			t1.Merge(t2)

			logrus.Debug("t1+2", t1)

			var buf3 bytes.Buffer
			var buf4 bytes.Buffer
			t3.Serialize(&buf3)
			t1.Serialize(&buf4)
			Expect(buf4).To(Equal(buf3))
		})
	})
})
