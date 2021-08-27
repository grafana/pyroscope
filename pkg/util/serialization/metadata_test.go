package serialization

import (
	"bufio"
	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("metadata", func() {
	in := map[string]interface{}{
		"foo": 1.0,
		"bar": "baz",
	}
	out := "\x15{\"bar\":\"baz\",\"foo\":1}"

	Describe("WriteMetadata", func() {
		It("serializes metadata", func() {
			b := &bytes.Buffer{}
			WriteMetadata(b, in)
			Expect(b.String()).To(Equal(out))
		})
	})

	Describe("ReadMetadata", func() {
		It("deserializes metadata", func() {
			b := bufio.NewReader(bytes.NewReader([]byte(out)))
			res, err := ReadMetadata(b)
			Expect(err).ToNot(HaveOccurred())
			Expect(res["foo"]).To(Equal(in["foo"]))
			Expect(res["bar"]).To(Equal(in["bar"]))
		})
	})
})
