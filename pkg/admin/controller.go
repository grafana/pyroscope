package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type Controller struct {
	log *logrus.Logger

	adminService *AdminService
	userService  UserService
}

type UserService interface {
	UpdateUserByName(ctx context.Context, name string, params model.UpdateUserParams) (model.User, error)
}

func NewController(log *logrus.Logger, adminService *AdminService, userService UserService) *Controller {
	return &Controller{
		log: log,

		adminService: adminService,
		userService:  userService,
	}
}

// HandleGetApps handles GET requests
func (ctrl *Controller) HandleGetApps(w http.ResponseWriter, _ *http.Request) {
	appNames := ctrl.adminService.GetApps()

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

	err = ctrl.adminService.DeleteApp(payload.Name)
	if err != nil {
		// TODO how to distinguish
		// it was a bad request
		// or an internal server error
		ctrl.writeError(w, http.StatusInternalServerError, err, "")
		return
	}

	w.WriteHeader(200)
}

type UpdateUserRequest struct {
	Password   *string `json:"password"`
	IsDisabled *bool   `json:"isDisabled"`
}

func (r *UpdateUserRequest) SetIsDisabled(v bool) { r.IsDisabled = &v }

func (ctrl *Controller) UpdateUserHandler(w http.ResponseWriter, r *http.Request) {
	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ctrl.writeError(w, http.StatusBadRequest, err, "failed to unmarshal JSON")
		return
	}

	params := model.UpdateUserParams{
		Password:   req.Password,
		IsDisabled: req.IsDisabled,
	}

	username := mux.Vars(r)["username"]
	if _, err := ctrl.userService.UpdateUserByName(r.Context(), username, params); err != nil {
		ctrl.writeError(w, http.StatusInternalServerError, err, fmt.Sprintf("can't update user %s", username))
	}
}
