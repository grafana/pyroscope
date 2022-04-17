package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/sirupsen/logrus"
)

//go:generate mockgen -destination mocks/user.go -package mocks . UserService

type UserService interface {
	CreateUser(context.Context, model.CreateUserParams) (model.User, error)
	FindUserByID(context.Context, uint) (model.User, error)
	GetAllUsers(context.Context) ([]model.User, error)
	UpdateUserByID(context.Context, uint, model.UpdateUserParams) (model.User, error)
	UpdateUserPasswordByID(context.Context, uint, model.UpdateUserPasswordParams) error
	DeleteUserByID(context.Context, uint) error
}

type UserHandler struct {
	logger      logrus.FieldLogger
	userService UserService
	httpUtils   httputils.Helper
}

func NewUserHandler(logger logrus.FieldLogger, userService UserService, httpUtils httputils.Helper) UserHandler {
	return UserHandler{
		logger:      logger,
		userService: userService,
		httpUtils:   httpUtils,
	}
}

type user struct {
	ID                uint       `json:"id"`
	Name              string     `json:"name"`
	Email             *string    `json:"email,omitempty"`
	FullName          *string    `json:"fullName,omitempty"`
	Role              model.Role `json:"role"`
	IsDisabled        bool       `json:"isDisabled"`
	IsExternal        bool       `json:"isExternal"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
	LastSeenAt        *time.Time `json:"lastSeenAt,omitempty"`
	PasswordChangedAt time.Time  `json:"passwordChangedAt"`
}

type createUserRequest struct {
	Name     string     `json:"name"`
	Email    *string    `json:"email,omitempty"`
	FullName *string    `json:"fullName,omitempty"`
	Password []byte     `json:"password"`
	Role     model.Role `json:"role"`
}

type updateUserRequest struct {
	Name     *string `json:"name,omitempty"`
	Email    *string `json:"email,omitempty"`
	FullName *string `json:"fullName,omitempty"`
}

type resetUserPasswordRequest struct {
	Password []byte `json:"password"`
}

type changeUserPasswordRequest struct {
	OldPassword []byte `json:"oldPassword"`
	NewPassword []byte `json:"newPassword"`
}

type changeUserRoleRequest struct {
	Role model.Role `json:"role"`
}

func userFromModel(u model.User) user {
	return user{
		ID:                u.ID,
		Name:              u.Name,
		Email:             u.Email,
		FullName:          u.FullName,
		Role:              u.Role,
		IsDisabled:        model.IsUserDisabled(u),
		IsExternal:        model.IsUserExternal(u),
		PasswordChangedAt: u.PasswordChangedAt,
		LastSeenAt:        u.LastSeenAt,
		CreatedAt:         u.CreatedAt,
		UpdatedAt:         u.UpdatedAt,
	}
}

func (h UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpUtils.HandleError(w, r, h.logger, httputils.JSONError{Err: err})
		return
	}
	params := model.CreateUserParams{
		Name:     req.Name,
		Email:    req.Email,
		FullName: req.FullName,
		Password: string(req.Password),
		Role:     req.Role,
	}
	u, err := h.userService.CreateUser(r.Context(), params)
	if err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	h.httpUtils.MustJSON(w, userFromModel(u))
}

func (h UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := h.httpUtils.IdFromRequest(r)
	if err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	u, err := h.userService.FindUserByID(r.Context(), id)
	if err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	h.httpUtils.MustJSON(w, userFromModel(u))
}

func (h UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	u, err := h.userService.GetAllUsers(r.Context())
	if err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	users := make([]user, len(u))
	for i := range u {
		users[i] = userFromModel(u[i])
	}
	h.httpUtils.MustJSON(w, users)
}

func (h UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := h.httpUtils.IdFromRequest(r)
	if err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	h.updateUser(w, r, id)
}

func (h UserHandler) updateUser(w http.ResponseWriter, r *http.Request, id uint) {
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpUtils.HandleError(w, r, h.logger, httputils.JSONError{Err: err})
		return
	}
	params := model.UpdateUserParams{
		Name:     req.Name,
		Email:    req.Email,
		FullName: req.FullName,
	}
	u, err := h.userService.UpdateUserByID(r.Context(), id, params)
	if err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	h.httpUtils.MustJSON(w, userFromModel(u))
}

func (h UserHandler) ChangeUserPassword(w http.ResponseWriter, r *http.Request) {
	id, err := h.httpUtils.IdFromRequest(r)
	if err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	if isSameUser(r.Context(), id) {
		h.httpUtils.HandleError(w, r, h.logger, model.ErrPermissionDenied)
		return
	}
	var req resetUserPasswordRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpUtils.HandleError(w, r, h.logger, httputils.JSONError{Err: err})
		return
	}
	params := model.UpdateUserParams{Password: model.String(string(req.Password))}
	if _, err = h.userService.UpdateUserByID(r.Context(), id, params); err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h UserHandler) ChangeUserRole(w http.ResponseWriter, r *http.Request) {
	id, err := h.httpUtils.IdFromRequest(r)
	if err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	if isSameUser(r.Context(), id) {
		h.httpUtils.HandleError(w, r, h.logger, model.ErrPermissionDenied)
		return
	}
	var req changeUserRoleRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpUtils.HandleError(w, r, h.logger, httputils.JSONError{Err: err})
		return
	}
	params := model.UpdateUserParams{Role: &req.Role}
	if _, err = h.userService.UpdateUserByID(r.Context(), id, params); err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h UserHandler) DisableUser(w http.ResponseWriter, r *http.Request) {
	h.setUserDisabled(w, r, true)
}

func (h UserHandler) EnableUser(w http.ResponseWriter, r *http.Request) {
	h.setUserDisabled(w, r, false)
}

func (h UserHandler) setUserDisabled(w http.ResponseWriter, r *http.Request, disabled bool) {
	id, err := h.httpUtils.IdFromRequest(r)
	if err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	if isSameUser(r.Context(), id) {
		h.httpUtils.HandleError(w, r, h.logger, model.ErrPermissionDenied)
		return
	}
	params := model.UpdateUserParams{IsDisabled: &disabled}
	if _, err = h.userService.UpdateUserByID(r.Context(), id, params); err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := h.httpUtils.IdFromRequest(r)
	if err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	if err = h.userService.DeleteUserByID(r.Context(), id); err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h UserHandler) GetAuthenticatedUser(w http.ResponseWriter, r *http.Request) {
	u, ok := model.UserFromContext(r.Context())
	if !ok {
		h.httpUtils.HandleError(w, r, h.logger, model.ErrPermissionDenied)
		return
	}
	h.httpUtils.MustJSON(w, userFromModel(u))
}

func (h UserHandler) UpdateAuthenticatedUser(w http.ResponseWriter, r *http.Request) {
	u, ok := model.UserFromContext(r.Context())
	if !ok {
		h.httpUtils.HandleError(w, r, h.logger, model.ErrPermissionDenied)
		return
	}
	h.updateUser(w, r, u.ID)
}

func (h UserHandler) ChangeAuthenticatedUserPassword(w http.ResponseWriter, r *http.Request) {
	u, ok := model.UserFromContext(r.Context())
	if !ok {
		h.httpUtils.HandleError(w, r, h.logger, model.ErrPermissionDenied)
		return
	}
	var req changeUserPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpUtils.HandleError(w, r, h.logger, httputils.JSONError{Err: err})
		return
	}
	params := model.UpdateUserPasswordParams{
		OldPassword: string(req.OldPassword),
		NewPassword: string(req.NewPassword),
	}
	if err := h.userService.UpdateUserPasswordByID(r.Context(), u.ID, params); err != nil {
		h.httpUtils.HandleError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func isSameUser(ctx context.Context, id uint) bool {
	if u, ok := model.UserFromContext(ctx); ok {
		return id == u.ID
	}
	return false
}
