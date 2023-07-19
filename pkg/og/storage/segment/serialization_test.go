package segment

import (
	"bytes"
	"log"
	"math/big"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/grafana/pyroscope/pkg/og/testing"
)

var serializedExampleV1 = "\x01({\"sampleRate\":0,\"spyName\":\"\",\"units\":\"\"}" +
	"\x01\x80\x92\xb8Ø\xfe\xff\xff\xff\x01\x03\x01\x03\x00\x80\x92\xb8Ø\xfe\xff\xff" +
	"\xff\x01\x01\x01\x00\x00\x8a\x92\xb8Ø\xfe\xff\xff\xff\x01\x01\x01\x00\x00\x94\x92" +
	"\xb8Ø\xfe\xff\xff\xff\x01\x01\x01\x00"

var serializedExampleV2 = "\x02={\"aggregationType\":\"\",\"sampleRate\":0,\"spyName\":\"\",\"units\":\"\"}" +
	"\x01\x80\x92\xb8Ø\xfe\xff\xff\xff\x01\x03\x03\x01\x03\x00\x80\x92\xb8Ø\xfe\xff\xff\xff\x01\x01\x01\x01\x00" +
	"\x00\x8a\x92\xb8Ø\xfe\xff\xff\xff\x01\x01\x01\x01\x00\x00\x94\x92\xb8Ø\xfe\xff\xff\xff\x01\x01\x01\x01\x00"

var serializedExampleV3 = "\x03={\"aggregationType\":\"\",\"sampleRate\":0,\"spyName\":\"\",\"units\":\"\"}" +
	"\x01\x80\x92\xb8Ø\xfe\xff\xff\xff\x01\x03\x03\x01\x03\x00\x80\x92\xb8Ø\xfe\xff\xff\xff\x01\x01\x01\x01\x00" +
	"\x00\x8a\x92\xb8Ø\xfe\xff\xff\xff\x01\x01\x01\x01\x00\x00\x94\x92\xb8Ø\xfe\xff\xff\xff\x01\x01\x01\x01\x00" +
	"\x80\x92\xb8Ø\xfe\xff\xff\xff\x01\x00"

var _ = Describe("stree", func() {
	Context("Serialize / Deserialize", func() {
		It("both functions work properly", func() {
			s := New()
			s.Put(testing.SimpleTime(0),
				testing.SimpleTime(9), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
			s.Put(testing.SimpleTime(10),
				testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
			s.Put(testing.SimpleTime(20),
				testing.SimpleTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})

			s.watermarks = watermarks{absoluteTime: testing.SimpleTime(100)}

			var buf bytes.Buffer
			s.Serialize(&buf)
			serialized := buf.Bytes()
			log.Printf("%q", serialized)

			s, err := Deserialize(bytes.NewReader(serialized))
			Expect(err).ToNot(HaveOccurred())
			var buf2 bytes.Buffer
			s.Serialize(&buf2)
			serialized2 := buf2.Bytes()
			Expect(string(serialized2)).To(Equal(string(serialized)))
		})
	})

	Context("Serialize", func() {
		It("serializes segment tree properly", func() {
			s := New()
			s.Put(testing.SimpleTime(0),
				testing.SimpleTime(9), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
			s.Put(testing.SimpleTime(10),
				testing.SimpleTime(19), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})
			s.Put(testing.SimpleTime(20),
				testing.SimpleTime(29), 1, func(de int, t time.Time, r *big.Rat, a []Addon) {})

			var buf bytes.Buffer
			s.Serialize(&buf)
			serialized := buf.Bytes()
			log.Printf("q: %q", string(serialized))
			Expect(string(serialized)).To(Equal(serializedExampleV3))
		})
	})

	Context("Deserialize", func() {
		Context("v1", func() {
			It("deserializes v1 data", func() {
				s, err := Deserialize(bytes.NewReader([]byte(serializedExampleV1)))
				Expect(err).ToNot(HaveOccurred())
				Expect(s.root.children[0]).ToNot(BeNil())
				Expect(s.root.children[1]).ToNot(BeNil())
				Expect(s.root.children[2]).ToNot(BeNil())
				Expect(s.root.children[3]).To(BeNil())
			})
		})
		Context("v2", func() {
			It("deserializes v2 data", func() {
				s, err := Deserialize(bytes.NewReader([]byte(serializedExampleV2)))
				Expect(err).ToNot(HaveOccurred())
				Expect(s.root.children[0]).ToNot(BeNil())
				Expect(s.root.children[1]).ToNot(BeNil())
				Expect(s.root.children[2]).ToNot(BeNil())
				Expect(s.root.children[3]).To(BeNil())
				Expect(s.root.writes).To(Equal(uint64(3)))
			})
		})
		Context("v3", func() {
			It("deserializes v3 data", func() {
				s, err := Deserialize(bytes.NewReader([]byte(serializedExampleV3)))
				Expect(err).ToNot(HaveOccurred())
				Expect(s.root.children[0]).ToNot(BeNil())
				Expect(s.root.children[1]).ToNot(BeNil())
				Expect(s.root.children[2]).ToNot(BeNil())
				Expect(s.root.children[3]).To(BeNil())
				Expect(s.root.writes).To(Equal(uint64(3)))
			})
		})
	})

	Context("watermarks serialize / deserialize", func() {
		It("both functions work properly", func() {
			w := watermarks{
				absoluteTime: testing.SimpleTime(100),
				levels: map[int]time.Time{
					0: testing.SimpleTime(100),
					1: testing.SimpleTime(1000),
				},
			}

			var buf bytes.Buffer
			err := w.serialize(&buf)
			Expect(err).ToNot(HaveOccurred())

			s := New()
			err = deserializeWatermarks(bytes.NewReader(buf.Bytes()), &s.watermarks)
			Expect(err).ToNot(HaveOccurred())
			Expect(w).To(Equal(s.watermarks))
		})
	})
})
