package service_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/model"
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
		FullyQualifiedName: "myapp",
		SampleRate:         100,
		SpyName:            "gospy",
		Units:              metadata.SamplesUnits,
		AggregationType:    metadata.AverageAggregationType,
	}

	assertNumOfApps := func(num int) []storage.Application {
		apps, err := svc.List(context.TODO())
		Expect(err).ToNot(HaveOccurred())
		Expect(len(apps)).To(Equal(num))
		return apps
	}

	It("validates input", func() {
		ctx := context.TODO()

		// Create
		err := svc.CreateOrUpdate(ctx, storage.Application{FullyQualifiedName: ""})
		Expect(model.IsValidationError(err)).To(BeTrue())

		// Get
		_, err = svc.Get(ctx, "")
		Expect(model.IsValidationError(err)).To(BeTrue())

		// Delete
		err = svc.Delete(ctx, "")
		Expect(model.IsValidationError(err)).To(BeTrue())
	})

	Context("create/update", func() {
		It("creates correctly", func() {
			ctx := context.TODO()
			assertNumOfApps(0)

			err := svc.CreateOrUpdate(ctx, storage.Application{})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(model.ErrApplicationNameEmpty))

			assertNumOfApps(0)
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
				FullyQualifiedName: app.FullyQualifiedName,
				SampleRate:         101,
			})
			Expect(err).ToNot(HaveOccurred())

			a, err := svc.Get(ctx, app.FullyQualifiedName)
			Expect(err).ToNot(HaveOccurred())

			// Other fields should not be touched
			app2 := app
			app2.SampleRate = 101
			Expect(a).To(Equal(app2))
		})

	})

	Context("get", func() {
		It("fetches correctly", func() {
			ctx := context.TODO()
			err := svc.CreateOrUpdate(ctx, app)
			Expect(err).ToNot(HaveOccurred())

			res, err := svc.Get(ctx, app.FullyQualifiedName)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(app))
		})
		It("fails when app doesn't exist", func() {
			ctx := context.TODO()
			_, err := svc.Get(ctx, "non_existing_app")
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(model.ErrApplicationNotFound))
		})
	})

	Context("delete", func() {
		It("deletes correctly", func() {
			ctx := context.TODO()
			err := svc.CreateOrUpdate(ctx, app)
			Expect(err).ToNot(HaveOccurred())
			assertNumOfApps(1)

			err = svc.Delete(ctx, app.FullyQualifiedName)

			Expect(err).ToNot(HaveOccurred())
			assertNumOfApps(0)
		})

		It("doesn't fail when app doesn't exist", func() {
			ctx := context.TODO()
			err := svc.Delete(ctx, "non_existing_app")
			Expect(err).ToNot(HaveOccurred())
		})
	})

})
