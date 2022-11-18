package service_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/model/appmetadata"
	"github.com/pyroscope-io/pyroscope/pkg/service"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
)

type mockApplicationWriter struct {
	onWrite func(application appmetadata.ApplicationMetadata) error
}

func (svc *mockApplicationWriter) CreateOrUpdate(ctx context.Context, application appmetadata.ApplicationMetadata) error {
	return svc.onWrite(application)
}

var _ = Describe("ApplicationWriteCacheService", func() {
	s := new(testSuite)
	BeforeEach(s.BeforeEach)
	AfterEach(s.AfterEach)

	sampleApp := appmetadata.ApplicationMetadata{
		FQName:          "myapp",
		SampleRate:      100,
		SpyName:         "gospy",
		Units:           metadata.SamplesUnits,
		AggregationType: metadata.AverageAggregationType,
	}

	var m mockApplicationWriter
	var svc *service.ApplicationMetadataCacheService
	var cfg service.ApplicationMetadataCacheServiceConfig

	BeforeEach(func() {
		svc = service.NewApplicationMetadataCacheService(cfg, &m)
	})

	When("cache is empty", func() {
		BeforeEach(func() {
			m = mockApplicationWriter{
				onWrite: func(application appmetadata.ApplicationMetadata) error {
					Expect(application).To(Equal(sampleApp))
					return nil
				},
			}
		})

		It("writes to underlying svc", func() {
			err := svc.CreateOrUpdate(context.TODO(), sampleApp)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	When("cache is not empty", func() {
		var n int

		BeforeEach(func() {
			n = 0
			m = mockApplicationWriter{
				onWrite: func(application appmetadata.ApplicationMetadata) error {
					n = n + 1
					return nil
				},
			}
		})

		When("data is the same", func() {
			It("does not write to underlying svc", func() {
				err := svc.CreateOrUpdate(context.TODO(), sampleApp)
				Expect(err).ToNot(HaveOccurred())

				err = svc.CreateOrUpdate(context.TODO(), sampleApp)
				Expect(err).ToNot(HaveOccurred())
				Expect(n).To(Equal(1))
			})
		})

		When("data is different", func() {
			It("writes to the underlying svc", func() {
				err := svc.CreateOrUpdate(context.TODO(), sampleApp)
				Expect(err).ToNot(HaveOccurred())

				sampleApp2 := sampleApp
				sampleApp2.SpyName = "myspy"

				err = svc.CreateOrUpdate(context.TODO(), sampleApp2)
				Expect(err).ToNot(HaveOccurred())
				Expect(n).To(Equal(2))
			})
		})
	})
})
