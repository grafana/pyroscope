package api_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/service"
)

type mockService struct {
	createAnnotationResponse func() (*model.Annotation, error)
}

func (m *mockService) CreateAnnotation(ctx context.Context, params service.CreateAnnotationParams) (*model.Annotation, error) {
	return m.createAnnotationResponse()
}

var _ = Describe("AnnotationCtrl", func() {
	It("works", func() {
		Expect(true).To(Equal(true))
	})
})
