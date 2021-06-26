package server

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (ctrl *Controller) configHandler(w http.ResponseWriter, r *http.Request) {
	configBytes, err := json.MarshalIndent(ctrl.config, "", "  ")
	if err != nil {
		renderServerError(w, fmt.Sprintf("could not marshal buildInfoObj json: %q", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(configBytes)
}
