package cireport_test

import (
	"context"
	"fmt"
	"time"

	"github.com/pyroscope-io/pyroscope/benchmark/internal/cireport"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tommy351/goldga"
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
			Expect(report).To(goldga.Match())
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
			Expect(report).To(goldga.Match())
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
			Expect(report).To(goldga.Match())
		})
	})
})
