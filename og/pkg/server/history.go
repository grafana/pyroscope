package server

import (
	"net/http"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/history"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
)

type HistoryHandler struct {
	log        *logrus.Logger
	httpUtils  httputils.Utils
	historyMgr history.Manager
}

func (ctrl *Controller) historyHandler() http.HandlerFunc {
	return NewHistoryHandler(ctrl.log, ctrl.httpUtils, ctrl.historyMgr).ServeHTTP
}

func NewHistoryHandler(
	l *logrus.Logger,
	httpUtils httputils.Utils,
	historyMgr history.Manager,
) *HistoryHandler {
	return &HistoryHandler{
		log:        l,
		httpUtils:  httpUtils,
		historyMgr: historyMgr,
	}
}

type response struct {
	Next    string           `json:"next"`
	History []*history.Entry `json:"history"`
}

func (rh *HistoryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	res, next, err := rh.historyMgr.List(r.Context(), r.URL.Query().Get("cursor"))
	if err != nil {
		rh.httpUtils.HandleError(r, w, err)
		return
	}
	rh.httpUtils.MustJSON(r, w, &response{
		Next:    next,
		History: res,
	})
}
