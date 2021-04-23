package segment

import (
	"bytes"
	"log"
	"math/big"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var serializedExampleV1 = "\x01({\"sampleRate\":0,\"spyName\":\"\",\"units\":\"\"}" +
	"\x01\x80\x92\xb8Ø\xfe\xff\xff\xff\x01\x03\x01\x03\x00\x80\x92\xb8Ø\xfe\xff\xff" +
	"\xff\x01\x01\x01\x00\x00\x8a\x92\xb8Ø\xfe\xff\xff\xff\x01\x01\x01\x00\x00\x94\x92" +
	"\xb8Ø\xfe\xff\xff\xff\x01\x01\x01\x00"

var serializedExampleV2 = "\x02({\"sampleRate\":0,\"spyName\":\"\",\"units\":\"\"}" +
	"\x01\x80\x92\xb8Ø\xfe\xff\xff\xff\x01\x03\x03\x01\x03\x00\x80\x92\xb8Ø\xfe\xff" +
	"\xff\xff\x01\x01\x01\x01\x00\x00\x8a\x92\xb8Ø\xfe\xff\xff\xff\x01\x01\x01\x01" +
	"\x00\x00\x94\x92\xb8Ø\xfe\xff\xff\xff\x01\x01\x01\x01\x00"

var _ = Describe("stree", func() {
	r := 10 * time.Second
	m := 10

	Context("Serialize / Deserialize", func() {
		It("both functions work properly", func() {
			s := New(r, m)
			s.Put(testing.SimpleTime(0),
				testing.SimpleTime(9), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
			s.Put(testing.SimpleTime(10),
				testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
			s.Put(testing.SimpleTime(20),
				testing.SimpleTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})

			var buf bytes.Buffer
			s.Serialize(&buf)
			serialized := buf.Bytes()
			log.Printf("%q", serialized)

			s, err := Deserialize(r, m, bytes.NewReader(serialized))
			Expect(err).ToNot(HaveOccurred())
			var buf2 bytes.Buffer
			s.Serialize(&buf2)
			serialized2 := buf2.Bytes()
			Expect(string(serialized2)).To(Equal(string(serialized)))
		})
	})

	Context("Serialize", func() {
		It("serializes segment tree properly", func() {
			s := New(r, m)
			s.Put(testing.SimpleTime(0),
				testing.SimpleTime(9), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
			s.Put(testing.SimpleTime(10),
				testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
			s.Put(testing.SimpleTime(20),
				testing.SimpleTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})

			var buf bytes.Buffer
			s.Serialize(&buf)
			serialized := buf.Bytes()
			Expect(string(serialized)).To(Equal(serializedExampleV2))
		})
	})

	Context("Deserialize", func() {
		Context("v1", func() {
			It("deserializes v1 data", func() {
				s, err := Deserialize(r, m, bytes.NewReader([]byte(serializedExampleV1)))
				Expect(err).ToNot(HaveOccurred())
				Expect(s.root.children[0]).ToNot(BeNil())
				Expect(s.root.children[1]).ToNot(BeNil())
				Expect(s.root.children[2]).ToNot(BeNil())
				Expect(s.root.children[3]).To(BeNil())
			})
		})
		Context("v2", func() {
			It("deserializes v2 data", func() {
				s, err := Deserialize(r, m, bytes.NewReader([]byte(serializedExampleV2)))
				Expect(err).ToNot(HaveOccurred())
				Expect(s.root.children[0]).ToNot(BeNil())
				Expect(s.root.children[1]).ToNot(BeNil())
				Expect(s.root.children[2]).ToNot(BeNil())
				Expect(s.root.children[3]).To(BeNil())
				Expect(s.root.writes).To(Equal(uint64(3)))
			})
		})
	})
})
