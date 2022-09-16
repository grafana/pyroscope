package service_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/model"
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

			annotation, err := svc.CreateAnnotation(context.Background(), model.CreateAnnotation{
				AppName:   "myapp",
				Content:   "mycontent",
				Timestamp: now,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(annotation).ToNot(BeNil())
			Expect(annotation.AppName).To(Equal("myapp"))
			Expect(annotation.Content).To(Equal("mycontent"))
			Expect(annotation.Timestamp.Unix()).To(Equal(now.Unix()))

			Expect(annotation.CreatedAt).ToNot(BeZero())
			Expect(annotation.UpdatedAt).ToNot(BeZero())
		})

		It("validates parameters", func() {
			annotation, err := svc.CreateAnnotation(context.Background(), model.CreateAnnotation{})

			Expect(err).To(HaveOccurred())
			Expect(annotation).To(BeNil())
		})

		When("an annotation already exists", func() {
			p := model.CreateAnnotation{
				AppName:   "myapp",
				Content:   "mycontent",
				Timestamp: time.Now(),
			}

			BeforeEach(func() {
				annotation, err := svc.CreateAnnotation(context.Background(), p)
				Expect(err).ToNot(HaveOccurred())
				Expect(annotation).ToNot(BeNil())
			})

			It("upserts", func() {
				annotation, err := svc.CreateAnnotation(context.Background(), model.CreateAnnotation{
					AppName:   p.AppName,
					Timestamp: p.Timestamp,
					Content:   "mycontent updated",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(annotation).ToNot(BeNil())

				annotations, err := svc.FindAnnotationsByTimeRange(
					context.Background(), "myapp",
					time.Now().Add(-time.Hour),
					time.Now())

				Expect(annotations[0].Content).To(Equal("mycontent updated"))
				Expect(err).ToNot(HaveOccurred())
				Expect(annotations).ToNot(BeEmpty())
				Expect(len(annotations)).To(Equal(1))
			})
		})

	})

	Describe("find annotations", func() {
		When("there's no annotation", func() {
			It("returns empty array", func() {
				now := time.Now()

				annotations, err := svc.FindAnnotationsByTimeRange(context.Background(), "myapp", now, now)
				Expect(err).ToNot(HaveOccurred())
				Expect(annotations).To(BeEmpty())
			})
		})

		When("annotation exists", func() {
			var now time.Time

			BeforeEach(func() {
				now = time.Now()
				_, err := svc.CreateAnnotation(context.Background(), model.CreateAnnotation{
					AppName:   "myapp",
					Content:   "mycontent",
					Timestamp: now,
				})
				Expect(err).ToNot(HaveOccurred())
			})

			When("finding within interval", func() {
				It("finds correctly", func() {
					annotations, err := svc.FindAnnotationsByTimeRange(context.Background(), "myapp", now, now)
					Expect(err).ToNot(HaveOccurred())
					Expect(annotations).ToNot(BeEmpty())
					Expect(len(annotations)).To(Equal(1))
				})
			})

			When("finding outside the interval", func() {
				It("returns empty array", func() {
					annotations, err := svc.FindAnnotationsByTimeRange(context.Background(), "myapp",
						time.Now().Add(time.Hour),
						time.Now().Add(time.Hour*2),
					)

					Expect(err).ToNot(HaveOccurred())
					Expect(annotations).To(BeEmpty())
				})
			})
		})
	})
})
