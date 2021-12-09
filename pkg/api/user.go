package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/internal/model"
)

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

type userDTO struct {
	ID                uint       `json:"id"`
	FullName          string     `json:"full_name,omitempty"`
	Email             string     `json:"email"`
	Role              model.Role `json:"role"`
	IsDisabled        bool       `json:"is_disabled"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	LastSeenAt        time.Time  `json:"last_seen_at,omitempty"`
	PasswordChangedAt time.Time  `json:"password_changed_at"`
}

type createUserRequest struct {
	FullName *string    `json:"full_name,omitempty"`
	Email    string     `json:"email"`
	Password []byte     `json:"password"`
	Role     model.Role `json:"role"`
}

type updateUserRequest struct {
	FullName *string     `json:"full_name,omitempty"`
	Email    *string     `json:"email"`
	Role     *model.Role `json:"role"`
}

type changeUserPasswordRequest struct {
	Password []byte `json:"password"`
}

func userFromModel(u model.User) userDTO {
	return userDTO{
		ID:                u.ID,
		FullName:          u.FullName,
		Email:             u.Email,
		Role:              u.Role,
		IsDisabled:        model.IsUserDisabled(u),
		CreatedAt:         u.CreatedAt,
		UpdatedAt:         u.UpdatedAt,
		LastSeenAt:        u.LastSeenAt,
		PasswordChangedAt: u.PasswordChangedAt,
	}
}

func (h UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		DecodeError(w, err)
		return
	}
	user, err := h.userService.CreateUser(r.Context(), model.CreateUserParams{
		FullName: req.FullName,
		Email:    req.Email,
		Password: string(req.Password),
		Role:     req.Role,
	})
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

func (h UserHandler) GetUsers(w http.ResponseWriter, r *http.Request) {
	u, err := h.userService.GetAllUsers(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	users := make([]userDTO, len(u))
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
	var req updateUserRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		DecodeError(w, err)
		return
	}
	params := model.UpdateUserParams{
		FullName: req.FullName,
		Email:    req.Email,
		Role:     req.Role,
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
	var req changeUserPasswordRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		DecodeError(w, err)
		return
	}
	params := model.UpdateUserParams{Password: model.String(string(req.Password))}
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
