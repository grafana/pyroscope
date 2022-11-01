package service_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/service"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
)

var _ = Describe("ApplicationService", func() {
	s := new(testSuite)
	BeforeEach(s.BeforeEach)
	AfterEach(s.AfterEach)

	var svc service.ApplicationService
	BeforeEach(func() {
		svc = service.NewApplicationService(s.DB())
	})

	app := storage.Application{
		Name:            "myapp",
		SampleRate:      100,
		SpyName:         "gospy",
		Units:           metadata.SamplesUnits,
		AggregationType: metadata.AverageAggregationType,
	}

	assertNumOfApps := func(num int) []storage.Application {
		apps, err := svc.List(context.TODO())
		Expect(err).ToNot(HaveOccurred())
		Expect(len(apps)).To(Equal(num))
		return apps
	}

	It("creates correctly", func() {
		ctx := context.TODO()
		assertNumOfApps(0)

		err := svc.CreateOrUpdate(ctx, app)
		Expect(err).ToNot(HaveOccurred())

		apps := assertNumOfApps(1)
		Expect(apps[0]).To(Equal(app))
	})

	It("upserts", func() {
		assertNumOfApps(0)

		ctx := context.TODO()
		err := svc.CreateOrUpdate(ctx, app)
		Expect(err).ToNot(HaveOccurred())

		err = svc.CreateOrUpdate(ctx, app)
		Expect(err).ToNot(HaveOccurred())
		assertNumOfApps(1)
	})

	It("handle partial updates", func() {
		ctx := context.TODO()
		err := svc.CreateOrUpdate(ctx, app)
		Expect(err).ToNot(HaveOccurred())

		err = svc.CreateOrUpdate(ctx, storage.Application{
			Name:       app.Name,
			SampleRate: 101,
		})
		Expect(err).ToNot(HaveOccurred())

		a, err := svc.Get(ctx, app.Name)
		Expect(err).ToNot(HaveOccurred())

		// Other fields should not be touched
		app2 := app
		app2.SampleRate = 101
		Expect(a).To(Equal(app2))
	})

	It("fetches correctly", func() {
		ctx := context.TODO()
		err := svc.CreateOrUpdate(ctx, app)
		Expect(err).ToNot(HaveOccurred())

		res, err := svc.Get(ctx, app.Name)
		Expect(err).ToNot(HaveOccurred())
		Expect(res).To(Equal(app))
	})

	It("deletes correctly", func() {
		ctx := context.TODO()
		err := svc.CreateOrUpdate(ctx, app)
		Expect(err).ToNot(HaveOccurred())
		assertNumOfApps(1)

		err = svc.Delete(ctx, app.Name)

		Expect(err).ToNot(HaveOccurred())
		assertNumOfApps(0)
	})

	//	It("fails to get non existent app", func() {
	//		ctx := context.TODO()
	//		res, err := svc.Get(ctx, "non_existing_app")
	//		Expect(err).ToNot(HaveOccurred())
	//		Expect(res).To(BeNil())
	//	})
})
