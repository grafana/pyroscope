package server

import (
	"net/http"

	"gopkg.in/yaml.v2"
)

type configResponse struct {
	Yaml string `json:"yaml"`
}

func (ctrl *Controller) configHandler(w http.ResponseWriter, _ *http.Request) {
	configBytes, err := yaml.Marshal(ctrl.config)
	if err != nil {
		WriteInternalServerError(ctrl.log, w, err, "failed to serialize configurtion")
		return
	}
	resp := configResponse{
		Yaml: string(configBytes),
	}
	WriteResponseJSON(ctrl.log, w, resp)
}
