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
	AppName   []string `json:"appName"`
	Timestamp int64    `json:"timestamp"`
	Content   string   `json:"content"`
}

func (h *AnnotationsHandler) CreateAnnotation(w http.ResponseWriter, r *http.Request) {
	var params CreateParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		h.httpUtils.WriteInternalServerError(r, w, err, "failed to unmarshal JSON")
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

	createAnnotations := func(ctx context.Context, params CreateParams) ([]annotationsResponse, error) {
		g, ctx := errgroup.WithContext(ctx)

		results := make([]annotationsResponse, len(params.AppName))
		for i, appName := range params.AppName {
			appName := appName
			i := i
			g.Go(func() error {
				var params CreateParams

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

	if len(params.AppName) <= 0 {
		h.httpUtils.HandleError(r, w, model.ValidationError{errors.New("at least one appName must be provided")})

		return
	}

	res, err := createAnnotations(r.Context(), params)
	if err != nil {
		h.httpUtils.HandleError(r, w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	h.httpUtils.WriteResponseJSON(r, w, res)
}
