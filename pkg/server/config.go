package server

import (
	"net/http"

	"gopkg.in/yaml.v2"
)

type configResponse struct {
	Yaml string `json:"yaml"`
}

func (ctrl *Controller) configHandler(w http.ResponseWriter, r *http.Request) {
	configBytes, err := yaml.Marshal(ctrl.config)
	if err != nil {
		ctrl.httpUtils.WriteInternalServerError(r, w, err, "failed to serialize configurtion")
		return
	}
	resp := configResponse{
		Yaml: string(configBytes),
	}
	ctrl.httpUtils.WriteResponseJSON(r, w, resp)
}
