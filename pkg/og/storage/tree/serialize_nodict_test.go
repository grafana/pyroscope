package tree

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
)

var serializationExample = []byte("\x00\x00\x01\x01a\x00\x02\x01b\x01\x00\x01c\x02\x00")

var _ = Describe("tree package", func() {

	Describe("DeserializeNoDict", func() {
		It("returns correct results", func() {
			r := bytes.NewReader(serializationExample)
			t, err := DeserializeNoDict(r)
			Expect(err).ToNot(HaveOccurred())

			Expect(string(t.root.Name)).To(Equal(""))
			Expect(string(t.root.ChildrenNodes[0].Name)).To(Equal("a"))
			Expect(string(t.root.ChildrenNodes[0].ChildrenNodes[0].Name)).To(Equal("b"))
			Expect(string(t.root.ChildrenNodes[0].ChildrenNodes[1].Name)).To(Equal("c"))
		})
	})
})
