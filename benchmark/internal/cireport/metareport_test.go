package cireport_test

import (
	"github.com/pyroscope-io/pyroscope/benchmark/internal/cireport"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tommy351/goldga"
)

var _ = Describe("metareport", func() {
	It("generates a markdown report correctly", func() {
		mr, err := cireport.NewMetaReport([]string{"execution", "seed"})
		Expect(err).NotTo(HaveOccurred())

		report, err := mr.Report([]string{"execution=5m", "seed=4"})
		Expect(err).ToNot(HaveOccurred())
		Expect(report).To(goldga.Match())
	})

	Context("error conditions", func() {
		It("should fail when there are no elements allowed", func() {
			mr, err := cireport.NewMetaReport([]string{})

			Expect(err).To(HaveOccurred())
			Expect(mr).To(BeNil())
		})

		It("should fail when no element is asked to be reported", func() {
			mr, err := cireport.NewMetaReport([]string{"allowed"})
			Expect(err).NotTo(HaveOccurred())

			_, err = mr.Report([]string{})
			Expect(err).To(HaveOccurred())
		})

		It("should fail when an element doesn't pass the allowlist", func() {
			mr, err := cireport.NewMetaReport([]string{"allowed"})
			Expect(err).NotTo(HaveOccurred())

			_, err = mr.Report([]string{"A=B"})
			Expect(err).To(HaveOccurred())
		})

		It("only accepts variables in the format A=B", func() {
			mr, err := cireport.NewMetaReport([]string{"A"})
			Expect(err).NotTo(HaveOccurred())

			_, err = mr.Report([]string{"A"})
			Expect(err).To(HaveOccurred())

			_, err = mr.Report([]string{"A="})
			Expect(err).To(HaveOccurred())

			_, err = mr.Report([]string{"=B"})
			Expect(err).To(HaveOccurred())

			_, err = mr.Report([]string{"A=B"})
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
