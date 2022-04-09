package server

import (
	"net/http"
)

type configResponse struct {
	Yaml string `json:"yaml"`
}

func (ctrl *Controller) configHandler(w http.ResponseWriter, _ *http.Request) {
	configBytes, err := ctrl.config.MarshalYAML()
	if err != nil {
		WriteJSONEncodeError(ctrl.log, w, err)
		return
	}
	resp := configResponse{
		Yaml: string(configBytes),
	}
	WriteResponseJSON(ctrl.log, w, resp)
}
