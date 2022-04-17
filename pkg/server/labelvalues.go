package server

import (
	"encoding/json"
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/sirupsen/logrus"
)

func (ctrl *Controller) labelValuesHandler() http.HandlerFunc {
	return NewLabelValuesHandler(ctrl.log, ctrl.storage, ctrl.httpUtils)
}

func NewLabelValuesHandler(log *logrus.Logger, s storage.LabelValuesGetter, httpUtils httputils.Helper) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		labelName := r.URL.Query().Get("label")
		query := r.URL.Query().Get("query")

		if labelName == "" {
			httpUtils.WriteInvalidParameterError(log, w, errLabelIsRequired)
			return
		}

		values := make([]string, 0)
		if query != "" {
			s.GetValuesByQuery(ctx, labelName, query, func(v string) bool {
				values = append(values, v)
				return true
			})
		} else {
			s.GetValues(ctx, labelName, func(v string) bool {
				values = append(values, v)
				return true
			})
		}

		b, err := json.Marshal(values)
		if err != nil {
			httpUtils.WriteJSONEncodeError(log, w, err)
			return
		}
		_, _ = w.Write(b)
	}
}
