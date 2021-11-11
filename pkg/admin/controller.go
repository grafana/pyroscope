package admin

import (
	"net/http"

	"github.com/sirupsen/logrus"
)

type Controller struct {
	svc *AdminService
	log *logrus.Logger
}

func NewController(log *logrus.Logger, svc *AdminService) *Controller {
	ctrl := &Controller{
		svc,
		log,
	}

	return ctrl
}

func (ctrl *Controller) HandleGetApps(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		{
			appNames := ctrl.svc.GetAppNames()

			w.WriteHeader(200)
			ctrl.writeResponseJSON(w, appNames)
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
