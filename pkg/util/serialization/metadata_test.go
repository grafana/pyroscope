package serialization

import (
	"bufio"
	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("metadata", func() {
	in := map[string]interface{}{
		"foo": 1,
		"bar": "baz",
	}
	out := "\x15{\"bar\":\"baz\",\"foo\":1}"

	Describe("WriteMetadata", func() {
		It("serializes metadata", func() {
			b := &bytes.Buffer{}
			WriteMetadata(b, in)
			Expect(string(b.Bytes())).To(Equal(out))
		})
	})

	Describe("ReadMetadata", func() {
		It("deserializes metadata", func() {
			b := bufio.NewReader(bytes.NewReader([]byte(out)))
			Expect(ReadMetadata(b)).To(BeIdenticalTo(in))
		})
	})
})
