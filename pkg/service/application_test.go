package service_test

import (
	"context"

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
		It("works", func() {
			_ = svc
			err := svc.CreateOrUpdate(context.TODO(), storage.Application{})
			Expect(err).ToNot(HaveOccurred())
			//	now := time.Now()

			//	annotation, err := svc.CreateAnnotation(context.Background(), model.CreateAnnotation{
			//		AppName:   "myapp",
			//		Content:   "mycontent",
			//		Timestamp: now,
			//	})

			Expect(true).To(Equal(false))
		})
	})

})
