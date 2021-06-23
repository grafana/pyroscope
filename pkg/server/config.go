package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func (ctrl *Controller) configHandler(w http.ResponseWriter, r *http.Request) {
	configStr, err := configJSONString(ctrl.config)
	if err != nil {
		renderServerError(w, fmt.Sprintf("could not marshal buildInfoObj json: %q", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write([]byte(configStr))
}

func configJSONString(config *config.Server) (string, error) {
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}

	return string(b), nil
}
