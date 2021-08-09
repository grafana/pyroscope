package server

import (
	"encoding/json"
	"net/http"
)

func (ctrl *Controller) labelsHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")

	keys := make([]string, 0)
	if query != "" {
		ctrl.storage.GetKeysByQuery(query, func(k string) bool {
			keys = append(keys, k)
			return true
		})
	} else {
		ctrl.storage.GetKeys(func(k string) bool {
			keys = append(keys, k)
			return true
		})
	}

	b, err := json.Marshal(keys)
	if err != nil {
		ctrl.writeJSONEncodeError(w, err)
		return
	}
	_, _ = w.Write(b)
}

func (ctrl *Controller) labelValuesHandler(w http.ResponseWriter, r *http.Request) {
	labelName := r.URL.Query().Get("label")
	query := r.URL.Query().Get("query")

	if labelName == "" {
		ctrl.writeInvalidParameterError(w, errLabelIsRequired)
		return
	}

	values := make([]string, 0)
	if query != "" {
		ctrl.storage.GetValuesByQuery(labelName, query, func(v string) bool {
			values = append(values, v)
			return true
		})
	} else {
		ctrl.storage.GetValues(labelName, func(v string) bool {
			values = append(values, v)
			return true
		})
	}

	b, err := json.Marshal(values)
	if err != nil {
		ctrl.writeJSONEncodeError(w, err)
		return
	}
	_, _ = w.Write(b)
}
