package server

import (
	"net/http"
)

func (ctrl *Controller) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("server is ready"))
}
