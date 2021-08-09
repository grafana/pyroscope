package convert

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("convert", func() {
	Describe("ParseTrie", func() {
		It("parses data correctly", func() {
			r := bytes.NewReader([]byte("\x00\x00\x01\x06foo;ba\x00\x02\x01r\x01\x00\x01z\x01\x00"))
			result := []string{}
			ParseTrie(r, func(name []byte, val int) {
				result = append(result, fmt.Sprintf("%s %d", name, val))
			})
			Expect(result).To(ConsistOf("foo;bar 1", "foo;baz 1"))
		})
	})

	Describe("ParsePprof", func() {
		It("parses data correctly", func() {
			result := []string{}

			b, err := ioutil.ReadFile("testdata/cpu.pprof")
			Expect(err).ToNot(HaveOccurred())
			r := bytes.NewReader(b)
			g, err := gzip.NewReader(r)
			Expect(err).ToNot(HaveOccurred())
			p, err := ParsePprof(g)
			Expect(err).ToNot(HaveOccurred())

			p.Get("samples", func(name []byte, val int) {
				result = append(result, fmt.Sprintf("%s %d", name, val))
			})
			Expect(result).To(ContainElement("runtime.main;main.work 1"))
		})
	})

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
