package convert

import (
	"bytes"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("convert", func() {
	Describe("ParseGroups", func() {
		It("parses data correctly", func() {
			r := bytes.NewReader([]byte("foo;bar 10\nfoo;baz 20\n"))
			result := []string{}
			ParseGroups(r, func(name []byte, val int) {
				result = append(result, fmt.Sprintf("%s %d", name, val))
			})
			Expect(result).To(ConsistOf("foo;bar 10", "foo;baz 20"))
		})
	})

	Describe("ParseIndividualLines", func() {
		It("parses data correctly", func() {
			r := bytes.NewReader([]byte("foo;bar\nfoo;baz\n"))
			result := []string{}
			ParseIndividualLines(r, func(name []byte, val int) {
				result = append(result, fmt.Sprintf("%s %d", name, val))
			})
			Expect(result).To(ConsistOf("foo;bar 1", "foo;baz 1"))
		})
	})
})
