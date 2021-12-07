package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/internal/model"
	"github.com/pyroscope-io/pyroscope/pkg/internal/service"
)

type UserHandler struct {
	userService service.UserService
}

type User struct {
	ID                uint       `json:"id"`
	FullName          string     `json:"full_name,omitempty"`
	Email             string     `json:"email"`
	Role              model.Role `json:"role"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	LastSeenAt        time.Time  `json:"last_seen_at,omitempty"`
	PasswordChangedAt time.Time  `json:"password_changed_at"`
}

type CreateUserRequest struct {
	FullName *string    `json:"full_name,omitempty"`
	Email    string     `json:"email"`
	Password []byte     `json:"password"`
	Role     model.Role `json:"role"`
}

type UpdateUserRequest struct {
	FullName *string     `json:"full_name,omitempty"`
	Email    *string     `json:"email"`
	Role     *model.Role `json:"role"`
}

type ChangeUserPassword struct {
	Password []byte `json:"password"`
}

func UserFromModel(u *model.User) User {
	return User{
		ID:                u.ID,
		FullName:          u.FullName,
		Email:             u.Email,
		Role:              u.Role,
		CreatedAt:         u.CreatedAt,
		UpdatedAt:         u.UpdatedAt,
		LastSeenAt:        u.LastSeenAt,
		PasswordChangedAt: u.PasswordChangedAt,
	}
}

func (h UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// TODO: message
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	user, err := h.userService.CreateUser(r.Context(), model.CreateUserParams{
		FullName: req.FullName,
		Email:    req.Email,
		Password: req.Password,
		Role:     req.Role,
	})
	if err != nil {
		respondWithError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	respondWithJSON(w, UserFromModel(user))
}

func (h UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		respondWithError(w, err)
		return
	}
	user, err := h.userService.FindUserByID(r.Context(), id)
	if err != nil {
		respondWithError(w, err)
		return
	}
	respondWithJSON(w, UserFromModel(user))
}

func (h UserHandler) GetUsers(w http.ResponseWriter, r *http.Request) {
	u, err := h.userService.GetAllUsers(r.Context())
	if err != nil {
		respondWithError(w, err)
		return
	}
	users := make([]User, len(u))
	for i := range u {
		users[i] = UserFromModel(u[i])
	}
	respondWithJSON(w, users)
}

func (h UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		respondWithError(w, err)
		return
	}
	var req UpdateUserRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	user, err := h.userService.UpdateUserByID(r.Context(), id, model.UpdateUserParams{
		FullName: req.FullName,
		Email:    req.Email,
		Role:     req.Role,
	})
	if err != nil {
		respondWithError(w, err)
		return
	}
	respondWithJSON(w, UserFromModel(user))
}

func (h UserHandler) ChangeUserPassword(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		respondWithError(w, err)
		return
	}
	var req ChangeUserPassword
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	params := model.ChangeUserPassword{Password: req.Password}
	user, err := h.userService.ChangeUserPasswordByID(r.Context(), id, params)
	if err != nil {
		respondWithError(w, err)
		return
	}
	respondWithJSON(w, UserFromModel(user))
}

func (h UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		respondWithError(w, err)
		return
	}
	if err = h.userService.DeleteUserByID(r.Context(), id); err != nil {
		respondWithError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
