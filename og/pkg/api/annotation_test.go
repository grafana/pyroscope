package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/api/router"
	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/sirupsen/logrus"
)

type mockService struct {
	createAnnotationResponse func(params model.CreateAnnotation) (*model.Annotation, error)
}

func (m *mockService) CreateAnnotation(ctx context.Context, params model.CreateAnnotation) (*model.Annotation, error) {
	return m.createAnnotationResponse(params)
}

func (m *mockService) FindAnnotationsByTimeRange(ctx context.Context, appName string, startTime time.Time, endTime time.Time) ([]model.Annotation, error) {
	return []model.Annotation{}, nil
}

var _ = Describe("AnnotationHandler", func() {
	var (
		server *httptest.Server
		svc    *mockService
	)

	AfterEach(func() {
		server.Close()
	})

	Describe("create annotation", func() {
		When("all parameters are set", func() {
			BeforeEach(func() {
				svc = &mockService{
					createAnnotationResponse: func(params model.CreateAnnotation) (*model.Annotation, error) {
						return &model.Annotation{
							AppName:   "myApp",
							Content:   "mycontent",
							Timestamp: time.Unix(1662729099, 0),
						}, nil
					},
				}

				server = httptest.NewServer(newTestRouter(defaultUserCtx, router.Services{
					Logger:             logrus.StandardLogger(),
					AnnotationsService: svc,
				}))
			})

			It("creates correctly", func() {
				url := server.URL + "/annotations"

				expectResponse(newRequest(http.MethodPost, url,
					"annotation/create_request.json"),
					"annotation/create_response.json",
					http.StatusCreated)
			})
		})

		When("timestamp is absent", func() {
			It("it defaults to zero-value", func() {
				svc = &mockService{
					createAnnotationResponse: func(params model.CreateAnnotation) (m *model.Annotation, err error) {
						// The service should receive a zero timestamp
						Expect(params.Timestamp.IsZero()).To(BeTrue())

						return &model.Annotation{
							AppName:   "myApp",
							Content:   "mycontent",
							Timestamp: time.Unix(1662729099, 0),
						}, nil
					},
				}

				server = httptest.NewServer(newTestRouter(defaultUserCtx, router.Services{
					Logger:             logrus.StandardLogger(),
					AnnotationsService: svc,
				}))

				url := server.URL + "/annotations"

				expectResponse(newRequest(http.MethodPost, url,
					"annotation/create_request_no_timestamp.json"),
					"annotation/create_response.json",
					http.StatusCreated)
			})
		})

		When("multiple appNames are passed", func() {
			BeforeEach(func() {
				svc = &mockService{
					createAnnotationResponse: func(params model.CreateAnnotation) (*model.Annotation, error) {
						return &model.Annotation{
							AppName:   params.AppName,
							Content:   "mycontent",
							Timestamp: time.Unix(1662729099, 0),
						}, nil
					},
				}

				server = httptest.NewServer(newTestRouter(defaultUserCtx, router.Services{
					Logger:             logrus.StandardLogger(),
					AnnotationsService: svc,
				}))
			})

			It("creates correctly", func() {
				url := server.URL + "/annotations"

				expectResponse(newRequest(http.MethodPost, url,
					"annotation/create_multiple_request.json"),
					"annotation/create_multiple_response.json",
					http.StatusCreated)
			})
		})

		When("fields are invalid", func() {
			BeforeEach(func() {
				svc = &mockService{
					createAnnotationResponse: func(params model.CreateAnnotation) (*model.Annotation, error) {
						return nil, model.ValidationError{errors.New("myerror")}
					},
				}

				server = httptest.NewServer(newTestRouter(defaultUserCtx, router.Services{
					Logger:             logrus.StandardLogger(),
					AnnotationsService: svc,
				}))
			})
			It("returns an error", func() {
				url := server.URL + "/annotations"

				expectResponse(newRequest(http.MethodPost, url,
					"annotation/create_request_error.json"),
					"annotation/create_response_error.json",
					http.StatusBadRequest)
			})
		})

		When("both 'appName' and 'appNames' are passed", func() {
			BeforeEach(func() {
				svc = &mockService{
					createAnnotationResponse: func(params model.CreateAnnotation) (*model.Annotation, error) {
						return nil, nil
					},
				}

				server = httptest.NewServer(newTestRouter(defaultUserCtx, router.Services{
					Logger:             logrus.StandardLogger(),
					AnnotationsService: svc,
				}))
			})
			It("returns an error", func() {
				url := server.URL + "/annotations"

				expectResponse(newRequest(http.MethodPost, url,
					"annotation/create_request_appName_appNames_error.json"),
					"annotation/create_response_appName_appNames_error.json",
					http.StatusBadRequest)
			})
		})

		When("none of 'appName' and 'appNames' are passed", func() {
			BeforeEach(func() {
				svc = &mockService{
					createAnnotationResponse: func(params model.CreateAnnotation) (*model.Annotation, error) {
						return nil, nil
					},
				}

				server = httptest.NewServer(newTestRouter(defaultUserCtx, router.Services{
					Logger:             logrus.StandardLogger(),
					AnnotationsService: svc,
				}))
			})
			It("returns an error", func() {
				url := server.URL + "/annotations"

				expectResponse(newRequest(http.MethodPost, url,
					"annotation/create_request_appName_appNames_empty_error.json"),
					"annotation/create_response_appName_appNames_empty_error.json",
					http.StatusBadRequest)
			})
		})
	})
})
