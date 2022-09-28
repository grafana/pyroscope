package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
	"golang.org/x/sync/errgroup"
)

type AnnotationsService interface {
	CreateAnnotation(ctx context.Context, params model.CreateAnnotation) (*model.Annotation, error)
	FindAnnotationsByTimeRange(ctx context.Context, appName string, startTime time.Time, endTime time.Time) ([]model.Annotation, error)
}
type AnnotationsHandler struct {
	svc       AnnotationsService
	httpUtils httputils.Utils
}

func NewAnnotationsHandler(svc AnnotationsService, httpUtils httputils.Utils) *AnnotationsHandler {
	return &AnnotationsHandler{
		svc:       svc,
		httpUtils: httpUtils,
	}
}

type CreateParams struct {
	AppName   string   `json:"appName"`
	AppNames  []string `json:"appNames"`
	Timestamp int64    `json:"timestamp"`
	Content   string   `json:"content"`
}

func (h *AnnotationsHandler) CreateAnnotation(w http.ResponseWriter, r *http.Request) {
	params := h.validateAppNames(w, r)
	if params == nil {
		return
	}

	var timestamp time.Time
	if params.Timestamp != 0 {
		timestamp = attime.Parse(strconv.FormatInt(params.Timestamp, 10))
	}

	// TODO(eh-am): unify this with render.go
	type annotationsResponse struct {
		AppName   string `json:"appName"`
		Content   string `json:"content"`
		Timestamp int64  `json:"timestamp"`
	}

	createAnnotations := func(ctx context.Context, params *CreateParams) ([]annotationsResponse, error) {
		g, ctx := errgroup.WithContext(ctx)

		results := make([]annotationsResponse, len(params.AppNames))
		for i, appName := range params.AppNames {
			appName := appName
			i := i
			g.Go(func() error {
				annotation, err := h.svc.CreateAnnotation(ctx, model.CreateAnnotation{
					AppName:   appName,
					Timestamp: timestamp,
					Content:   params.Content,
				})
				if err != nil {
					return err
				}

				results[i] = annotationsResponse{
					AppName:   annotation.AppName,
					Content:   annotation.Content,
					Timestamp: annotation.Timestamp.Unix(),
				}

				return err
			})
		}

		if err := g.Wait(); err != nil {
			return nil, err
		}
		return results, nil
	}

	res, err := createAnnotations(r.Context(), params)
	if err != nil {
		h.httpUtils.HandleError(r, w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	if len(res) == 1 {
		h.httpUtils.WriteResponseJSON(r, w, res[0])
		return
	}

	h.httpUtils.WriteResponseJSON(r, w, res)
}

// validateAppNames handles the different combinations between (`appName` and `appNames`)
// in the failure case, it returns nil and serves an error
// in the success case, it returns a `CreateParams` struct where `appNames` is ALWAYS populated with at least one appName
func (h *AnnotationsHandler) validateAppNames(w http.ResponseWriter, r *http.Request) *CreateParams {
	var params CreateParams

	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		h.httpUtils.WriteInternalServerError(r, w, err, "failed to unmarshal JSON")
		return nil
	}

	// handling `appName` and `appNames`
	// 1. Both are set, is invalid
	if params.AppName != "" && len(params.AppNames) > 0 {
		h.httpUtils.HandleError(r, w, model.ValidationError{errors.New("only one of 'appName' and 'appNames' can be specified")})
		return nil
	}

	// 2. None are set
	if params.AppName == "" && len(params.AppNames) <= 0 {
		h.httpUtils.HandleError(r, w, model.ValidationError{errors.New("at least one of 'appName' and 'appNames' needs to be specified")})
		return nil
	}

	// 3. Only appName is set
	if params.AppName != "" && len(params.AppNames) <= 0 {
		params.AppNames = append(params.AppNames, params.AppName)
		return &params
	}

	// 4. Only appNames is set
	return &params
}
