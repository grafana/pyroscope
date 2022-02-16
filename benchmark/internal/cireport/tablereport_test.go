package cireport_test

import (
	"context"
	"fmt"
	"time"

	"github.com/pyroscope-io/pyroscope/benchmark/internal/cireport"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type mockQuerier struct {
	i          int
	respQueryA float64
	respQueryB float64
}

func (m *mockQuerier) Instant(query string, t time.Time) (float64, error) {
	fmt.Println(m.i)
	m.i = m.i + 1
	fmt.Println(m.i)

	switch m.i {
	case 1:
		return m.respQueryA, nil
	case 2:
		return m.respQueryB, nil
	}
	panic("not implemented")
}

var _ = Describe("tablereport", func() {
	qc := cireport.QueriesConfig{
		BaseName:   "base name",
		TargetName: "target name",
		Queries: []cireport.Query{
			{
				Name:           "my name",
				Description:    "my description",
				Base:           "query_base",
				Target:         "query_target",
				DiffThreshold:  5,
				BiggerIsBetter: true,
			},
		},
	}

	Context("diff within threshold", func() {
		It("should generate a report correctly", func() {
			q := &mockQuerier{
				respQueryA: 1,
				respQueryB: 1,
			}

			tr := cireport.NewTableReport(q)
			report, err := tr.Report(context.Background(), &qc)

			Expect(err).ToNot(HaveOccurred())
			Expect(report).To(BeEquivalentTo("## Result\n|             | base name           | target name | diff                | threshold |\n|-------------|------------------------------|----------------------|---------------------|----------------|\n| my name |  1.00 | 1.00  |  0.00 (0.00%) | 5% |\n\n\n\n<details>\n  <summary>Details</summary>\n\n\n| Name     | Description | Query for base name   | Query for target name |\n|----------|-------------|----------------------|----------------------|\n| my name | my description | `query_base` | `query_target`  |\n\n\n</details>\n"))
		})
	})

	Context("diff bigger than threshold", func() {
		It("should generate a report correctly", func() {
			q := &mockQuerier{
				respQueryA: 0,
				respQueryB: 2,
			}

			tr := cireport.NewTableReport(q)
			report, err := tr.Report(context.Background(), &qc)

			Expect(err).ToNot(HaveOccurred())
			Expect(report).To(BeEquivalentTo("## Result\n|             | base name           | target name | diff                | threshold |\n|-------------|------------------------------|----------------------|---------------------|----------------|\n| my name |  0.00 | 2.00  |  :green_square: 2.00 (200.00%) | 5% |\n\n\n\n<details>\n  <summary>Details</summary>\n\n\n| Name     | Description | Query for base name   | Query for target name |\n|----------|-------------|----------------------|----------------------|\n| my name | my description | `query_base` | `query_target`  |\n\n\n</details>\n"))
		})
	})

	Context("diff smaller than threshold", func() {
		It("should generate a report correctly", func() {
			q := &mockQuerier{
				respQueryA: 2,
				respQueryB: 0,
			}

			tr := cireport.NewTableReport(q)
			report, err := tr.Report(context.Background(), &qc)

			Expect(err).ToNot(HaveOccurred())
			Expect(report).To(BeEquivalentTo("## Result\n|             | base name           | target name | diff                | threshold |\n|-------------|------------------------------|----------------------|---------------------|----------------|\n| my name |  2.00 | 0.00  |  :red_square: -2.00 (-200.00%) | 5% |\n\n\n\n<details>\n  <summary>Details</summary>\n\n\n| Name     | Description | Query for base name   | Query for target name |\n|----------|-------------|----------------------|----------------------|\n| my name | my description | `query_base` | `query_target`  |\n\n\n</details>\n"))
		})
	})
})
