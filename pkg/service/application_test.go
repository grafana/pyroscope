package service_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/service"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

var _ = Describe("ApplicationService", func() {
	s := new(testSuite)
	BeforeEach(s.BeforeEach)
	AfterEach(s.AfterEach)

	var svc service.ApplicationService
	BeforeEach(func() {
		svc = service.NewApplicationService(s.DB())
	})

	Describe("create application", func() {
		It("creates correctly", func() {
			ctx := context.TODO()
			apps, err := svc.List(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(apps)).To(Equal(0))

			err = svc.CreateOrUpdate(ctx, storage.Application{
				Name: "myapp",
			})
			Expect(err).ToNot(HaveOccurred())
			apps, err = svc.List(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(apps)).To(Equal(1))
			fmt.Println("apps0", apps[0])
			Expect(apps[0]).To(Equal(storage.Application{
				Name: "myapp",
			}))
		})

		It("upserts", func() {
			ctx := context.TODO()
			err := svc.CreateOrUpdate(ctx, storage.Application{
				Name: "myapp",
			})
			Expect(err).ToNot(HaveOccurred())

			err = svc.CreateOrUpdate(ctx, storage.Application{
				Name: "myapp",
			})
			Expect(err).ToNot(HaveOccurred())

			apps, err := svc.List(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(apps)).To(Equal(1))
		})

		//		It("does not allow empty name", func() {
		//
		//		})
	})

})
