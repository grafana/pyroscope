package cireport_test

import (
	"github.com/pyroscope-io/pyroscope/benchmark/internal/cireport"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("metareport", func() {
	It("generates a markdown report correctly", func() {
		mr, err := cireport.NewMetaReport([]string{"execution", "seed"})
		Expect(err).NotTo(HaveOccurred())

		report, err := mr.Report("Server Benchmark", []string{"execution=5m", "seed=4"})
		Expect(err).ToNot(HaveOccurred())

		Expect(report).To(BeEquivalentTo("## Server Benchmark\n\n<details>\n  <summary>Details</summary>\n\n\n|  Name       | Value      |\n|-------------|------------|\n| `execution` | `5m` |\n| `seed` | `4` |\n</details>\n"))
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

			_, err = mr.Report("Server Benchmark", []string{})
			Expect(err).To(HaveOccurred())
		})

		It("should fail when an element doesn't pass the allowlist", func() {
			mr, err := cireport.NewMetaReport([]string{"allowed"})
			Expect(err).NotTo(HaveOccurred())

			_, err = mr.Report("Server Benchmark", []string{"A=B"})
			Expect(err).To(HaveOccurred())
		})

		It("only accepts variables in the format A=B", func() {
			mr, err := cireport.NewMetaReport([]string{"A"})
			Expect(err).NotTo(HaveOccurred())

			_, err = mr.Report("Server Benchmark", []string{"A"})
			Expect(err).To(HaveOccurred())

			_, err = mr.Report("Server Benchmark", []string{"A="})
			Expect(err).To(HaveOccurred())

			_, err = mr.Report("Server Benchmark", []string{"=B"})
			Expect(err).To(HaveOccurred())

			_, err = mr.Report("Server Benchmark", []string{"A=B"})
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
