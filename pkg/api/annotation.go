package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
	"github.com/sirupsen/logrus"
)

type AnnotationsService interface {
	CreateAnnotation(ctx context.Context, params model.CreateAnnotation) (*model.Annotation, error)
}
type AnnotationsHandler struct {
	logger    logrus.FieldLogger
	svc       AnnotationsService
	httpUtils httputils.Utils
}

func NewAnnotationsHandler(logger logrus.FieldLogger, svc AnnotationsService, httpUtils httputils.Utils) *AnnotationsHandler {
	return &AnnotationsHandler{
		logger:    logger,
		svc:       svc,
		httpUtils: httpUtils,
	}
}

type CreateParams struct {
	AppName   string `json:"appName"`
	Timestamp int64  `json:"timestamp"`
	Content   string `json:"content"`
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

	annotation, err := h.svc.CreateAnnotation(r.Context(), model.CreateAnnotation{
		AppName:   params.AppName,
		Timestamp: timestamp,
		Content:   params.Content,
	})
	if err != nil {
		h.httpUtils.HandleError(r, w, err)
		return
	}

	// TODO(eh-am): unify this with render.go
	type annotationsResponse struct {
		AppName   string `json:"appName"`
		Content   string `json:"content"`
		Timestamp int64  `json:"timestamp"`
	}
	annotationsResp := annotationsResponse{
		AppName:   annotation.AppName,
		Content:   annotation.Content,
		Timestamp: annotation.Timestamp.Unix(),
	}

	w.WriteHeader(http.StatusCreated)
	h.httpUtils.WriteResponseJSON(r, w, annotationsResp)
}
