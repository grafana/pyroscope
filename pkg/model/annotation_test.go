package model_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/model"
)

var _ = Describe("Annotation", func() {
	Describe("CreateAnnotation", func() {
		When("required fields are missing", func() {
			It("fails with multiple errors", func() {
				m := model.CreateAnnotation{}
				Expect(m.Parse()).To(MatchError(model.ErrAnnotationInvalidAppName))
				Expect(m.Parse()).To(MatchError(model.ErrAnnotationInvalidContent))
			})
		})

		When("timestamp is absent", func() {
			It("defaults to time.Now()", func() {
				m := model.CreateAnnotation{
					AppName: "myappname",
					Content: "mycontent",
				}
				Expect(m.Parse()).ToNot(HaveOccurred())

				// Instead of mocking time.Now, it's easier to just assert it's not zero
				Expect(m.Timestamp).ToNot(BeZero())
			})
		})

		When("appName contains tags", func() {
			It("errors", func() {
				m := model.CreateAnnotation{
					AppName: `myappname{my="tag"}`,
					Content: "mycontent",
				}
				Expect(m.Parse()).ToNot(HaveOccurred())
				Expect(m.AppName).To(Equal("myappname"))
			})
		})

		When("appName is a query without name", func() {
			It("errors", func() {
				m := model.CreateAnnotation{
					AppName: `{my="tag"}`,
					Content: "mycontent",
				}
				err := m.Parse()
				Expect(err).To(MatchError(model.ErrAnnotationInvalidAppName))
			})
		})
	})
})
