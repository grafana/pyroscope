package server

import (
	"encoding/json"
	"net/http"
)

func (ctrl *Controller) configHandler(w http.ResponseWriter, _ *http.Request) {
	configBytes, err := json.MarshalIndent(ctrl.config, "", "  ")
	if err != nil {
		WriteJSONEncodeError(ctrl.log, w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(configBytes)
}
