package service_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/service"
)

var _ = Describe("AnnotationsService", func() {
	s := new(testSuite)
	BeforeEach(s.BeforeEach)
	AfterEach(s.AfterEach)

	var svc service.AnnotationsService
	BeforeEach(func() {
		svc = service.NewAnnotationsService(s.DB())
	})

	Describe("create annotation", func() {
		It("works", func() {
			now := time.Now()
			annotation, err := svc.CreateAnnotation(context.Background(), service.CreateAnnotationParams{
				AppName:   "myapp",
				Content:   "mycontent",
				Timestamp: now,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(annotation).ToNot(BeNil())
			Expect(annotation.AppName).To(Equal("myapp"))
			Expect(annotation.Content).To(Equal("mycontent"))
			Expect(annotation.From).To(Equal(now))
		})
	})

	Describe("find annotations", func() {
		It("find within an interval", func() {
			now := time.Now()
			_, err := svc.CreateAnnotation(context.Background(), service.CreateAnnotationParams{
				AppName:   "myapp",
				Content:   "mycontent",
				Timestamp: now,
			})
			Expect(err).ToNot(HaveOccurred())

			annotations, err := svc.FindAnnotationsByTimeRange(context.Background(), "myapp", now, now)
			Expect(err).ToNot(HaveOccurred())
			Expect(annotations).ToNot(BeEmpty())
			Expect(len(annotations)).To(Equal(1))
		})
	})
})
