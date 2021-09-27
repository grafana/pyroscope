package tree

import (
	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
)

var v1NoDict = []byte("\x01\x00\x00\x01\x02\x00\x01\x00\x02\x02\x01\x01\x01\x00\x02\x02\x01\x02\x00")

var _ = Describe("tree", func() {
	Describe("Deserialize", func() {
		It("supports V1", func() {
			d := dict.New()
			r := bytes.NewReader(v1NoDict)
			t, err := Deserialize(d, r)
			Expect(err).ToNot(HaveOccurred())
			expectLabel(t, 0, "")
			expectLabel(t, t.root().ChildrenNodes[0], "label not found AAE=")
			expectLabel(t, t.at(t.root().ChildrenNodes[0]).ChildrenNodes[0], "label not found AQE=")
			expectLabel(t, t.at(t.root().ChildrenNodes[0]).ChildrenNodes[1], "label not found AgE=")
		})

		It("supports decoding w/o dictionary", func() {
			tree := New()
			tree.Insert([]byte("a;b"), uint64(1))
			tree.Insert([]byte("a;c"), uint64(2))
			expected := tree.String()

			var buf bytes.Buffer
			Expect(tree.SerializeV1NoDict(1024, &buf)).ToNot(HaveOccurred())
			dt, err := DeserializeV1NoDict(bytes.NewReader(buf.Bytes()))
			Expect(err).ToNot(HaveOccurred())
			Expect(dt.String()).To(Equal(expected))
		})
	})
})
