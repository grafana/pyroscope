package tree

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	"github.com/grafana/pyroscope/pkg/og/storage/dict"
)

var _ = Describe("truncation", func() {
	defer GinkgoRecover()

	BeforeEach(func() {
		// we override these two to better see what's going on
		lostDuringSerializationName = []byte("ls")
		lostDuringRenderingName = jsonableSlice("lr")
	})

	AfterEach(func() {
		lostDuringSerializationName = []byte("other")
		lostDuringRenderingName = jsonableSlice("other")
	})

	Context("small tree", func() {
		var treeA *Tree
		// treeA := New()
		BeforeEach(func() {
			treeA = New()
			treeA.Insert([]byte("a"), uint64(1))
			treeA.Insert([]byte("b"), uint64(2))
			treeA.Insert([]byte("c"), uint64(3))
		})

		Context("with dictionary", func() {
			d := dict.New()
			It("after serialization drops node 'a'", func() {
				buf := &bytes.Buffer{}
				treeA.SerializeTruncate(d, 3, buf)
				b := buf.Bytes()
				treeB, err := Deserialize(d, bytes.NewReader(b))
				Expect(err).ToNot(HaveOccurred())
				Expect(treeB.StringWithEmpty()).To(Equal(treeStr(`"b" 2|"c" 3|"ls" 1|`)))

				treeA.Insert([]byte("d"), uint64(1))

				buf = &bytes.Buffer{}
				treeA.SerializeTruncate(d, 3, buf)
				b = buf.Bytes()

				treeC, err := Deserialize(d, bytes.NewReader(b))
				Expect(err).ToNot(HaveOccurred())
				Expect(treeC.StringWithEmpty()).To(Equal(treeStr(`"b" 2|"c" 3|"ls" 2|`)))
				Expect(treeC.StringWithEmpty()).To(Equal(treeA.StringWithEmpty()))
			})
		})

		Context("without dictionary", func() {
			It("after serialization drops node 'a'", func() {
				buf := &bytes.Buffer{}
				treeA.SerializeTruncateNoDict(3, buf)
				treeB, err := DeserializeNoDict(buf)
				Expect(err).ToNot(HaveOccurred())
				Expect(treeB.StringWithEmpty()).To(Equal(treeStr(`"b" 2|"c" 3|"ls" 1|`)))
			})
		})
	})

	// TODO(petethepig): add more tests
})
