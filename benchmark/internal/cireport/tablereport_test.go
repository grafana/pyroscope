package cireport_test

import (
	"context"
	"github.com/pyroscope-io/pyroscope/benchmark/internal/cireport"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type mockQuerier struct{}

func (mockQuerier) Instant(query string, t time.Time) (float64, error) {
	return 0, nil
}

var _ = Describe("tablereport", func() {
	It("works", func() {
		q := mockQuerier{}
		tr := cireport.NewTableReport(q)

		queries := cireport.QueriesConfig{}
		tr.Report(context.Background(), &queries)

		Expect(true).To(Equal(false))
	})
})
