package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/service"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type AppInfo struct {
	Name string `json:"name"`
}

type DeleteAppInput struct {
	Name string `json:"name"`
}

func (ctrl *Controller) getAppsHandler() http.HandlerFunc {
	svc := service.NewApplicationMetadataService(ctrl.db)
	return NewGetAppsHandler(svc, ctrl.httpUtils)
}

type AppGetter interface {
	List(ctx context.Context) (apps []storage.ApplicationMetadata, err error)
}

func NewGetAppsHandler(s AppGetter, httpUtils httputils.Utils) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		apps, err := s.List(r.Context())
		if err != nil {
			httpUtils.HandleError(r, w, err)
			return
		}

		//		apps, err := s.GetApps(r.Context())
		//		if err != nil {
		//			httpUtils.WriteError(r, w, http.StatusInternalServerError, err, "")
		//			return
		//		}
		//		res := make([]AppInfo, 0, len(apps.Apps))
		//		for _, app := range apps.Apps {
		//			it := AppInfo{
		//				Name: app.Name,
		//			}
		//			res = append(res, it)
		//		}
		w.WriteHeader(http.StatusOK)
		httpUtils.WriteResponseJSON(r, w, apps)
	}
}

func (ctrl *Controller) getAppNames() http.HandlerFunc {
	return NewGetAppNamesHandler(ctrl.storage, ctrl.httpUtils)
}

func NewGetAppNamesHandler(s storage.AppNameGetter, httpUtils httputils.Utils) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		appNames := s.GetAppNames(r.Context())
		w.WriteHeader(http.StatusOK)
		httpUtils.WriteResponseJSON(r, w, appNames)
	}
}

func (ctrl *Controller) deleteAppsHandler() http.HandlerFunc {
	return NewDeleteAppHandler(ctrl.storage, ctrl.httpUtils)
}

func NewDeleteAppHandler(s storage.AppDeleter, httpUtils httputils.Utils) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload DeleteAppInput

		err := json.NewDecoder(r.Body).Decode(&payload)
		if err != nil {
			httpUtils.WriteError(r, w, http.StatusBadRequest, err, "")
			return
		}

		err = s.DeleteApp(r.Context(), payload.Name)
		if err != nil {
			// TODO how to distinguish
			// it was a bad request
			// or an internal server error
			httpUtils.WriteError(r, w, http.StatusInternalServerError, err, "")
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
