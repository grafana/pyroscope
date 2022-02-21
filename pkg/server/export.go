package server

import (
	"fmt"
	"io"
	"net/http"
)

type Payload struct {
	Name    string
	Profile string
	Type    string
}

func (ctrl *Controller) exportHandler(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Post("https://flamegraph.com/api/upload/v1", "application/json", r.Body)
	if err != nil {
		ctrl.writeError(w, 500, err, fmt.Sprintf("could not upload profile: %v", err))
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	io.Copy(w, resp.Body)
}
