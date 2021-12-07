package server

import (
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/internal/model"
)

type Authenticator interface {
	Authenticate(user *model.User) error
}

type LoginHandler struct{}

func (LoginHandler) Login(w http.ResponseWriter, r *http.Request) {
	// TODO(kolesnikovae): read form and authenticate user.
}
