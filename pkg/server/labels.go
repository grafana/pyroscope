package server

import (
	"encoding/json"
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

func (ctrl *Controller) labelsHandler() http.HandlerFunc {
	return NewLabelsHandler(ctrl.storage, ctrl.httpUtils).ServeHTTP
}

func NewLabelsHandler(s storage.LabelsGetter, httpUtils httputils.Utils) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		v := r.URL.Query()

		in := storage.GetLabelKeysByQueryInput{
			StartTime: attime.Parse(v.Get("from")),
			EndTime:   attime.Parse(v.Get("until")),
			Query:     v.Get("query"),
		}

		keys := make([]string, 0)
		if in.Query != "" {
			output, err := s.GetKeysByQuery(ctx, in)
			if err != nil {
				httpUtils.WriteInvalidParameterError(r, w, err)
				return
			}
			keys = append(keys, output.Keys...)
		} else {
			s.GetKeys(ctx, func(k string) bool {
				keys = append(keys, k)
				return true
			})
		}

		b, err := json.Marshal(keys)
		if err != nil {
			httpUtils.WriteJSONEncodeError(r, w, err)
			return
		}
		_, _ = w.Write(b)
	}
}
