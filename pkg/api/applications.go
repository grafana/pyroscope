package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type ApplicationService interface {
	List(context.Context) ([]storage.ApplicationMetadata, error)               // GET /apps
	Get(ctx context.Context, name string) (storage.ApplicationMetadata, error) // GET /apps/{name}
	CreateOrUpdate(context.Context, storage.ApplicationMetadata) error         // PUT /apps // Should be idempotent.
	Delete(ctx context.Context, name string) error                             // DELETE /apps/{name}
}

type ApplicationsHandler struct {
	svc       ApplicationService
	httpUtils httputils.Utils
}

func NewApplicationsHandler(svc ApplicationService, httpUtils httputils.Utils) *ApplicationsHandler {
	return &ApplicationsHandler{
		svc:       svc,
		httpUtils: httpUtils,
	}
}

func (h *ApplicationsHandler) GetApps(w http.ResponseWriter, r *http.Request) {
	apps, err := h.svc.List(r.Context())
	if err != nil {
		h.httpUtils.HandleError(r, w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	h.httpUtils.WriteResponseJSON(r, w, apps)
}

type DeleteAppInput struct {
	Name string `json:"name"`
}

func (h *ApplicationsHandler) DeleteApp(w http.ResponseWriter, r *http.Request) {
	var payload DeleteAppInput

	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		h.httpUtils.WriteError(r, w, http.StatusBadRequest, err, "")
		return
	}

	err = h.svc.Delete(r.Context(), payload.Name)
	if err != nil {
		// TODO how to distinguish
		// it was a bad request
		// or an internal server error
		h.httpUtils.WriteError(r, w, http.StatusInternalServerError, err, "")
		return
	}

	w.WriteHeader(http.StatusOK)
}
