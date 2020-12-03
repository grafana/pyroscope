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
	w.Write(b)
	w.WriteHeader(200)
}

func (ctrl *Controller) labelValuesHandler(w http.ResponseWriter, r *http.Request) {
	res := []string{}
	ctrl.s.GetValues(r.URL.Query().Get("label"), func(v string) bool {
		res = append(res, v)
		return true
	})
	b, err := json.Marshal(res)
	if err != nil {
		panic(err) // TODO: handle
	}
	w.Write(b)
	w.WriteHeader(200)
}
