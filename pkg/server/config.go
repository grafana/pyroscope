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
		ctrl.writeEncodeError(w, err)
		return
	}
	resp := configResponse{
		Yaml: string(configBytes),
	}
	ctrl.writeResponseJSON(w, resp)
}
