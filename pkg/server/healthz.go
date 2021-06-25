package server

import (
	"net/http"
)

func (*Controller) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("server is ready"))
}
