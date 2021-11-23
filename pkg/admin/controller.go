package admin

import (
	"encoding/json"
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

// HandleGetApps handles GET requests
func (ctrl *Controller) HandleGetApps(w http.ResponseWriter, _ *http.Request) {
	appNames := ctrl.svc.GetApps()

	w.WriteHeader(200)
	ctrl.writeResponseJSON(w, appNames)
}

type DeleteAppInput struct {
	Name string `json:"name"`
}

// HandleDeleteApp handles DELETE requests
func (ctrl *Controller) HandleDeleteApp(w http.ResponseWriter, r *http.Request) {
	var payload DeleteAppInput

	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		ctrl.writeError(w, http.StatusBadRequest, err, "")
		return
	}

	err = ctrl.svc.DeleteApp(payload.Name)
	if err != nil {
		// TODO how to distinguish
		// it was a bad request
		// or an internal server error
		ctrl.writeError(w, http.StatusInternalServerError, err, "")
		return
	}

	w.WriteHeader(200)
}
