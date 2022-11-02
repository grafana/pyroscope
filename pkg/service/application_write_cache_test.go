package service_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/service"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
)

type mockApplicationWriter struct {
	onWrite func(application storage.Application) error
}

func (svc mockApplicationWriter) CreateOrUpdate(ctx context.Context, application storage.Application) error {
	return svc.onWrite(application)
}

var _ = Describe("ApplicationWriteCacheService", func() {
	s := new(testSuite)
	BeforeEach(s.BeforeEach)
	AfterEach(s.AfterEach)

	//	var svc service.ApplicationCacheService
	BeforeEach(func() {
		//		svc = *service.NewApplicationCacheService()
	})

	sampleApp := storage.Application{
		Name:            "myapp",
		SampleRate:      100,
		SpyName:         "gospy",
		Units:           metadata.SamplesUnits,
		AggregationType: metadata.AverageAggregationType,
	}

	When("cache is empty", func() {
		It("writes to underlying svc", func() {
			m := mockApplicationWriter{
				onWrite: func(application storage.Application) error {
					Expect(application).To(Equal(sampleApp))
					return nil
				},
			}
			svc := service.NewApplicationCacheService(service.ApplicationCacheServiceConfig{}, m)

			err := svc.CreateOrUpdate(context.TODO(), sampleApp)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	FWhen("cache is not empty", func() {
		When("data is the same", func() {
			It("does not write to underlying svc", func() {
				n := 0
				m := mockApplicationWriter{
					onWrite: func(application storage.Application) error {
						n = n + 1
						return nil
					},
				}

				svc := service.NewApplicationCacheService(service.ApplicationCacheServiceConfig{}, m)
				err := svc.CreateOrUpdate(context.TODO(), sampleApp)
				Expect(err).ToNot(HaveOccurred())

				err = svc.CreateOrUpdate(context.TODO(), sampleApp)
				Expect(err).ToNot(HaveOccurred())
				Expect(n).To(Equal(1))
			})
		})
	})
})
