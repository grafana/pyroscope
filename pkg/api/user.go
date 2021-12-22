package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

//go:generate mockgen -destination mocks/user.go -package mocks . UserService

type UserService interface {
	CreateUser(context.Context, model.CreateUserParams) (model.User, error)
	FindUserByID(context.Context, uint) (model.User, error)
	GetAllUsers(context.Context) ([]model.User, error)
	UpdateUserByID(context.Context, uint, model.UpdateUserParams) (model.User, error)
	DeleteUserByID(context.Context, uint) error
}

type UserHandler struct {
	userService UserService
}

func NewUserHandler(userService UserService) UserHandler {
	return UserHandler{userService}
}

type User struct {
	ID                uint       `json:"id"`
	Name              string     `json:"name"`
	Email             string     `json:"email"`
	FullName          *string    `json:"fullName,omitempty"`
	IsDisabled        bool       `json:"isDisabled"`
	IsAdmin           bool       `json:"isAdmin"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
	LastSeenAt        *time.Time `json:"lastSeenAt,omitempty"`
	PasswordChangedAt time.Time  `json:"passwordChangedAt"`
}

type createUserRequest struct {
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	FullName *string `json:"fullName,omitempty"`
	Password []byte  `json:"password"`
	IsAdmin  bool    `json:"isAdmin"`
}

type updateUserRequest struct {
	Name     *string `json:"name,omitempty"`
	Email    *string `json:"email,omitempty"`
	FullName *string `json:"fullName,omitempty"`
}

type changeUserPasswordRequest struct {
	Password []byte `json:"password"`
}

type changeUserRolesRequest struct {
	IsAdmin *bool `json:"isAdmin"`
}

func userFromModel(u model.User) User {
	return User{
		ID:                u.ID,
		Name:              u.Name,
		Email:             u.Email,
		FullName:          u.FullName,
		IsDisabled:        model.IsUserDisabled(u),
		IsAdmin:           model.IsUserAdmin(u),
		PasswordChangedAt: u.PasswordChangedAt,
		LastSeenAt:        u.LastSeenAt,
		CreatedAt:         u.CreatedAt,
		UpdatedAt:         u.UpdatedAt,
	}
}

func (h UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		DecodeError(w, err)
		return
	}
	params := model.CreateUserParams{
		Name:     req.Name,
		Email:    req.Email,
		FullName: req.FullName,
		Password: string(req.Password),
		IsAdmin:  req.IsAdmin,
	}
	user, err := h.userService.CreateUser(r.Context(), params)
	if err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	MustJSON(w, userFromModel(user))
}

func (h UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		Error(w, err)
		return
	}
	user, err := h.userService.FindUserByID(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	MustJSON(w, userFromModel(user))
}

func (h UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	u, err := h.userService.GetAllUsers(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	users := make([]User, len(u))
	for i := range u {
		users[i] = userFromModel(u[i])
	}
	MustJSON(w, users)
}

func (h UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		Error(w, err)
		return
	}
	h.updateUser(w, r, id)
}

func (h UserHandler) updateUser(w http.ResponseWriter, r *http.Request, id uint) {
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		DecodeError(w, err)
		return
	}
	params := model.UpdateUserParams{
		Name:     req.Name,
		Email:    req.Email,
		FullName: req.FullName,
	}
	user, err := h.userService.UpdateUserByID(r.Context(), id, params)
	if err != nil {
		Error(w, err)
		return
	}
	MustJSON(w, userFromModel(user))
}

func (h UserHandler) ChangeUserPassword(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		Error(w, err)
		return
	}
	h.changeUserPassword(w, r, id)
}

func (h UserHandler) changeUserPassword(w http.ResponseWriter, r *http.Request, id uint) {
	var req changeUserPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		DecodeError(w, err)
		return
	}
	params := model.UpdateUserParams{Password: model.String(string(req.Password))}
	if _, err := h.userService.UpdateUserByID(r.Context(), id, params); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h UserHandler) ChangeUserRoles(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		Error(w, err)
		return
	}
	var req changeUserRolesRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		DecodeError(w, err)
		return
	}
	if req.IsAdmin == nil {
		// TODO(kolesnikovae): Before we add support for fully-fledged RBAC
		//  (and multi-tenancy, perhaps), the property must be set. Later we
		//  can safely extend the request model (in 'one of' fashion).
		Error(w, ErrRequestBodyInvalid)
		return
	}
	params := model.UpdateUserParams{IsAdmin: req.IsAdmin}
	if _, err = h.userService.UpdateUserByID(r.Context(), id, params); err != nil {
		Error(w, err)
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
	id, err := idFromRequest(r)
	if err != nil {
		Error(w, err)
		return
	}
	params := model.UpdateUserParams{IsDisabled: &disabled}
	if _, err = h.userService.UpdateUserByID(r.Context(), id, params); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		Error(w, err)
		return
	}
	if err = h.userService.DeleteUserByID(r.Context(), id); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h UserHandler) GetAuthenticatedUser(w http.ResponseWriter, r *http.Request) {
	user, ok := model.UserFromContext(r.Context())
	if !ok {
		Error(w, ErrAuthenticationRequired)
		return
	}
	MustJSON(w, userFromModel(user))
}

func (h UserHandler) UpdateAuthenticatedUser(w http.ResponseWriter, r *http.Request) {
	user, ok := model.UserFromContext(r.Context())
	if !ok {
		Error(w, ErrAuthenticationRequired)
		return
	}
	h.updateUser(w, r, user.ID)
}

func (h UserHandler) ChangeAuthenticatedUserPassword(w http.ResponseWriter, r *http.Request) {
	user, ok := model.UserFromContext(r.Context())
	if !ok {
		Error(w, ErrAuthenticationRequired)
		return
	}
	h.changeUserPassword(w, r, user.ID)
}
