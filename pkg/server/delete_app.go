package server

import (
	"net/http"
)

func (ctrl *Controller) deleteAppHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		app := r.URL.Query().Get("app")
		err := ctrl.storage.DeleteApp(r.Context(), app)
		if err != nil {
			ctrl.httpUtils.WriteJSONEncodeError(r, w, err)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}
}
