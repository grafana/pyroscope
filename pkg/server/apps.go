package server

import (
	"encoding/json"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"net/http"
)

type AppInfo struct {
	Name string `json:"name"`
}

type DeleteAppInput struct {
	Name string `json:"name"`
}

func (c *Controller) getAppsHandler() http.HandlerFunc {
	return NewGetAppsHandler(c.storage, c.httpUtils)
}

func NewGetAppsHandler(s storage.AppGetter, httpUtils httputils.Utils) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		apps := s.GetApps(r.Context())
		res := make([]AppInfo, 0, len(apps.Apps))
		for _, app := range apps.Apps {
			it := AppInfo{
				Name: app.Name,
			}
			res = append(res, it)
		}
		w.WriteHeader(http.StatusOK)
		httpUtils.WriteResponseJSON(r, w, res)
	}
}

func (c *Controller) getAppNames() http.HandlerFunc {
	return NewGetAppNamesHandler(c.storage, c.httpUtils)
}

func NewGetAppNamesHandler(s storage.AppGetter, httpUtils httputils.Utils) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		appNames := s.GetAppNames(r.Context())
		w.WriteHeader(http.StatusOK)
		httpUtils.WriteResponseJSON(r, w, appNames)
	}
}

func (c *Controller) deleteAppsHandler() http.HandlerFunc {
	return NewDeleteAppHandler(c.storage, c.httpUtils)
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
