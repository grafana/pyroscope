package server

import (
	"encoding/json"
	"net/http"
)

func (ctrl *Controller) labelsHandler(w http.ResponseWriter, _ *http.Request) {
	res := []string{}
	ctrl.storage.GetKeys(func(k string) bool {
		res = append(res, k)
		return true
	})
	b, err := json.Marshal(res)
	if err != nil {
		panic(err) // TODO: handle
	}
	w.Write(b)
}

func (ctrl *Controller) labelValuesHandler(w http.ResponseWriter, r *http.Request) {
	res := []string{}
	labelName := r.URL.Query().Get("label")
	ctrl.storage.GetValues(labelName, func(v string) bool {
		res = append(res, v)
		return true
	})
	b, err := json.Marshal(res)
	if err != nil {
		panic(err) // TODO: handle
	}
	w.Write(b)
}
