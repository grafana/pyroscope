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
	FullName          *string    `json:"full_name,omitempty"`
	Login             string     `json:"login"`
	Email             string     `json:"email"`
	Role              model.Role `json:"role"`
	IsDisabled        bool       `json:"is_disabled"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	LastSeenAt        *time.Time `json:"last_seen_at,omitempty"`
	PasswordChangedAt time.Time  `json:"password_changed_at"`
}

type createUserRequest struct {
	FullName *string    `json:"full_name,omitempty"`
	Login    string     `json:"login"`
	Email    string     `json:"email"`
	Password []byte     `json:"password"`
	Role     model.Role `json:"role"`
}

type updateUserRequest struct {
	FullName *string `json:"full_name,omitempty"`
	Email    *string `json:"email"`
}

type changeUserPasswordRequest struct {
	Password []byte `json:"password"`
}

type changeUserRoleRequest struct {
	Role model.Role `json:"role"`
}

func userFromModel(u model.User) User {
	return User{
		ID:                u.ID,
		FullName:          u.FullName,
		Login:             "", // TODO
		Email:             u.Email,
		Role:              u.Role,
		IsDisabled:        model.IsUserDisabled(u),
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
		FullName: req.FullName,
		Email:    req.Email,
		Password: string(req.Password),
		Role:     req.Role,
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
		FullName: req.FullName,
		Email:    req.Email,
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

func (h UserHandler) ChangeUserRole(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		Error(w, err)
		return
	}
	var req changeUserRoleRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		DecodeError(w, err)
		return
	}
	// TODO: Check that admin doesn't revoke it's own Admin role?
	params := model.UpdateUserParams{Role: &req.Role}
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
