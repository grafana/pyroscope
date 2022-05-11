package server

import (
	"encoding/json"
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

func (ctrl *Controller) labelValuesHandler() http.HandlerFunc {
	return NewLabelValuesHandler(ctrl.storage, ctrl.httpUtils)
}

func NewLabelValuesHandler(s storage.LabelValuesGetter, httpUtils httputils.Utils) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		v := r.URL.Query()

		in := storage.GetLabelValuesByQueryInput{
			StartTime: attime.Parse(v.Get("from")),
			EndTime:   attime.Parse(v.Get("until")),
			Label:     v.Get("label"),
			Query:     v.Get("query"),
		}

		if in.Label == "" {
			httpUtils.WriteInvalidParameterError(r, w, errLabelIsRequired)
			return
		}

		values := make([]string, 0)
		if in.Query != "" {
			output, err := s.GetValuesByQuery(ctx, in)
			if err != nil {
				httpUtils.WriteInvalidParameterError(r, w, err)
				return
			}
			values = append(values, output.Values...)
		} else {
			s.GetValues(ctx, in.Label, func(v string) bool {
				values = append(values, v)
				return true
			})
		}

		b, err := json.Marshal(values)
		if err != nil {
			httpUtils.WriteJSONEncodeError(r, w, err)
			return
		}
		_, _ = w.Write(b)
	}
}
