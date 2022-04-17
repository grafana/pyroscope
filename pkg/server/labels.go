package server

import (
	"encoding/json"
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

func (ctrl *Controller) labelsHandler() http.HandlerFunc {
	return NewLabelsHandler(ctrl.storage, ctrl.httpUtils).ServeHTTP
}

func NewLabelsHandler(s storage.LabelsGetter, httpUtils httputils.Utils) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		query := r.URL.Query().Get("query")

		keys := make([]string, 0)
		if query != "" {
			s.GetKeysByQuery(ctx, query, func(k string) bool {
				keys = append(keys, k)
				return true
			})
		} else {
			s.GetKeys(ctx, func(k string) bool {
				keys = append(keys, k)
				return true
			})
		}

		b, err := json.Marshal(keys)
		if err != nil {
			httpUtils.WriteJSONEncodeError(w, err)
			return
		}
		_, _ = w.Write(b)
	}
}
