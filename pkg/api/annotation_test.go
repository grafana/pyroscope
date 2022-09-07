package api_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type mockService struct {
	createAnnotationResponse func() (*model.CreateAnnotation, error)
}

func (m *mockService) CreateAnnotation(ctx context.Context, params model.CreateAnnotation) (*model.CreateAnnotation, error) {
	return m.createAnnotationResponse()
}

var _ = Describe("AnnotationCtrl", func() {
	It("works", func() {
		Expect(true).To(Equal(true))
	})
})
