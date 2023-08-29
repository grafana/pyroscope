package openapiv2

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"github.com/prometheus/common/version"
)

var (
	//go:embed gen/phlare.swagger.json
	openapiV2 []byte
)

type handler struct {
	data []byte
}

func (h handler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(h.data)

}

func Handler() (http.Handler, error) {
	// replace and app information in swagger json

	obj := make(map[string]interface{})
	err := json.Unmarshal(openapiV2, &obj)
	if err != nil {
		return nil, err
	}

	obj["info"] = map[string]interface{}{
		"title":   "Grafana Pyroscope",
		"version": version.Info(),
	}

	b, err := json.Marshal(&obj)
	if err != nil {
		return nil, err
	}

	return handler{data: b}, nil
}
