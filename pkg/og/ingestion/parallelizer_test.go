package ingestion_test

import (
	"context"
	"io/ioutil"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/grafana/pyroscope/pkg/og/ingestion"
	"github.com/grafana/pyroscope/pkg/og/storage/metadata"
	"github.com/grafana/pyroscope/pkg/og/storage/segment"
	"github.com/grafana/pyroscope/pkg/og/util/attime"
	"github.com/sirupsen/logrus"
)

type mockPutter struct {
	Fn func(context.Context, *ingestion.IngestInput) error
}

func (m mockPutter) Ingest(ctx context.Context, in *ingestion.IngestInput) error {
	return m.Fn(ctx, in)
}

var _ = Describe("Parallelizer", func() {
	It("calls Putters", func() {
		logger := logrus.New()
		logger.SetOutput(ioutil.Discard)

		var wg sync.WaitGroup

		pi := &ingestion.IngestInput{
			Metadata: ingestion.Metadata{
				Key: segment.NewKey(map[string]string{
					"__name__": "myapp",
				}),

				StartTime:       attime.Parse("1654110240"),
				EndTime:         attime.Parse("1654110250"),
				SampleRate:      100,
				SpyName:         "gospy",
				Units:           metadata.SamplesUnits,
				AggregationType: metadata.SumAggregationType,
			},
		}

		fn := func(ctx context.Context, putinput *ingestion.IngestInput) error {
			defer GinkgoRecover()

			Expect(putinput).To(Equal(pi))
			wg.Done()
			return nil
		}

		mock1 := mockPutter{Fn: fn}
		mock2 := mockPutter{Fn: fn}

		p := ingestion.NewParallelizer(logger, mock1, mock2)

		wg.Add(2)
		p.Ingest(context.TODO(), pi)
		wg.Wait()
	})
})
