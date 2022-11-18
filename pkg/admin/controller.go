package admin

//revive:disable:max-public-structs dependencies

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/model/appmetadata"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type Controller struct {
	log            *logrus.Logger
	httpUtils      httputils.Utils
	appService     ApplicationListerAndDeleter
	userService    UserService
	storageService StorageService
}

type UserService interface {
	UpdateUserByName(ctx context.Context, name string, params model.UpdateUserParams) (model.User, error)
}

type StorageService interface {
	Cleanup(ctx context.Context) error
}

type ApplicationListerAndDeleter interface {
	List(ctx context.Context) (apps []appmetadata.ApplicationMetadata, err error)
	Delete(ctx context.Context, name string) error
}

func NewController(
	log *logrus.Logger,
	appService ApplicationListerAndDeleter,
	userService UserService,
	storageService StorageService) *Controller {
	return &Controller{
		log: log,

		appService:     appService,
		userService:    userService,
		storageService: storageService,
	}
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
