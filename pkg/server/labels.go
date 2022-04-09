package server

import (
	"encoding/json"
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/sirupsen/logrus"
)

func (ctrl *Controller) labelsHandler() http.HandlerFunc {
	return NewLabelsHandler(ctrl.log, ctrl.storage).ServeHTTP
}

func NewLabelsHandler(log *logrus.Logger, s storage.LabelsGetter) http.HandlerFunc {
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
			WriteJSONEncodeError(log, w, err)
			return
		}
		_, _ = w.Write(b)
	}
}
