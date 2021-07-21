package server

import (
	"encoding/json"
	"net/http"
)

func (ctrl *Controller) labelsHandler(w http.ResponseWriter, _ *http.Request) {
	var keys []string
	ctrl.storage.GetKeys(func(k string) bool {
		keys = append(keys, k)
		return true
	})
	b, err := json.Marshal(keys)
	if err != nil {
		ctrl.writeJSONEncodeError(w, err)
		return
	}
	_, _ = w.Write(b)
}

func (ctrl *Controller) labelValuesHandler(w http.ResponseWriter, r *http.Request) {
	labelName := r.URL.Query().Get("label")
	if labelName == "" {
		ctrl.writeInvalidParameterError(w, errLabelIsRequired)
		return
	}
	var values []string
	ctrl.storage.GetValues(labelName, func(v string) bool {
		values = append(values, v)
		return true
	})
	b, err := json.Marshal(values)
	if err != nil {
		ctrl.writeJSONEncodeError(w, err)
		return
	}
	_, _ = w.Write(b)
}
