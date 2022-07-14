package admin

//revive:disable:max-public-structs dependencies

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

	adminService   *AdminService
	userService    UserService
	storageService StorageService
}

type UserService interface {
	UpdateUserByName(ctx context.Context, name string, params model.UpdateUserParams) (model.User, error)
}

type StorageService interface {
	Cleanup(ctx context.Context) error
}

func NewController(
	log *logrus.Logger,
	adminService *AdminService,
	userService UserService,
	storageService StorageService) *Controller {
	return &Controller{
		log: log,

		adminService:   adminService,
		userService:    userService,
		storageService: storageService,
	}
}

// HandleGetApps handles GET requests
func (ctrl *Controller) HandleGetApps(w http.ResponseWriter, _ *http.Request) {
	appNames := ctrl.adminService.GetApps()

	w.WriteHeader(http.StatusOK)
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

	w.WriteHeader(http.StatusOK)
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

func (ctrl *Controller) StorageCleanupHandler(w http.ResponseWriter, r *http.Request) {
	if err := ctrl.storageService.Cleanup(r.Context()); err != nil {
		ctrl.writeError(w, http.StatusInternalServerError, err, "failed to clean up storage")
	}
}
