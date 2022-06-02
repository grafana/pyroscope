package remotewrite_test

import (
	"context"
	"io/ioutil"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/remotewrite"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
	"github.com/sirupsen/logrus"
)

type mockPutter struct {
	Fn func(context.Context, *parser.PutInput) error
}

func (m mockPutter) Put(ctx context.Context, put *parser.PutInput) error {
	return m.Fn(ctx, put)
}

var _ = Describe("Paralellizer", func() {
	It("calls Putters", func() {
		logger := logrus.New()
		logger.SetOutput(ioutil.Discard)

		var wg sync.WaitGroup

		pi := &parser.PutInput{
			Key: segment.NewKey(map[string]string{
				"__name__": "myapp",
			}),

			StartTime:       attime.Parse("1654110240"),
			EndTime:         attime.Parse("1654110250"),
			SampleRate:      100,
			SpyName:         "gospy",
			Units:           metadata.SamplesUnits,
			AggregationType: metadata.SumAggregationType,
		}

		fn := func(ctx context.Context, putinput *parser.PutInput) error {
			defer GinkgoRecover()

			Expect(putinput).To(Equal(pi))
			wg.Done()
			return nil
		}

		mock1 := mockPutter{Fn: fn}
		mock2 := mockPutter{Fn: fn}

		p := remotewrite.NewParalellizer(logger, mock1, mock2)

		wg.Add(2)
		p.Put(context.TODO(), pi)
		wg.Wait()
	})

})
