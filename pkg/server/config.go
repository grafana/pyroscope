package server

import (
	"net/http"

	"gopkg.in/yaml.v2"
)

type configResponse struct {
	Yaml string `json:"yaml"`
}

func (ctrl *Controller) configHandler(w http.ResponseWriter, _ *http.Request) {
	config := *ctrl.config
	const sensitive = "<sensitive>"
	if config.Auth.JWTSecret != "" {
		config.Auth.JWTSecret = sensitive
	}
	if config.Auth.Github.ClientSecret != "" {
		config.Auth.Github.ClientSecret = sensitive
	}
	if config.Auth.Google.ClientSecret != "" {
		config.Auth.Google.ClientSecret = sensitive
	}
	if config.Auth.Gitlab.ClientSecret != "" {
		config.Auth.Gitlab.ClientSecret = sensitive
	}
	if config.Auth.Internal.AdminUser.Password != "" {
		config.Auth.Internal.AdminUser.Password = sensitive
	}

	configBytes, err := yaml.Marshal(config)
	if err != nil {
		ctrl.writeEncodeError(w, err)
		return
	}
	resp := configResponse{
		Yaml: string(configBytes),
	}
	ctrl.writeResponseJSON(w, resp)
}
