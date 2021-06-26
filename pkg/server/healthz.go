package server

import (
	"net/http"
)

func (ctrl *Controller) healthz(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("server is ready"))
}
