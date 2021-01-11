package server

import (
	"encoding/json"
	"net/http"
)

func (ctrl *Controller) labelsHandler(w http.ResponseWriter, r *http.Request) {
	res := []string{}
	ctrl.s.GetKeys(func(k string) bool {
		res = append(res, k)
		return true
	})
	b, err := json.Marshal(res)
	if err != nil {
		panic(err) // TODO: handle
	}
	w.WriteHeader(200)
	w.Write(b)
}

func (ctrl *Controller) labelValuesHandler(w http.ResponseWriter, r *http.Request) {
	res := []string{}
	labelName := r.URL.Query().Get("label")
	ctrl.s.GetValues(labelName, func(v string) bool {
		if labelName != "__name__" || !arrContains(ctrl.cfg.Server.HideApplications, v) {
			res = append(res, v)
		}
		return true
	})
	b, err := json.Marshal(res)
	if err != nil {
		panic(err) // TODO: handle
	}
	w.WriteHeader(200)
	w.Write(b)
}

func arrContains(arr []string, searchValue string) bool {
	for _, v := range arr {
		if searchValue == v {
			return true
		}
	}
	return false
}
