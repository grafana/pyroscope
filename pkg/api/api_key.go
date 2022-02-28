package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/sirupsen/logrus"
)

//go:generate mockgen -destination mocks/api_key.go -package mocks . APIKeyService

type APIKeyService interface {
	CreateAPIKey(context.Context, model.CreateAPIKeyParams) (model.APIKey, string, error)
	GetAllAPIKeys(context.Context) ([]model.APIKey, error)
	DeleteAPIKeyByID(context.Context, uint) error
}

type APIKeyHandler struct {
	logger        logrus.FieldLogger
	apiKeyService APIKeyService
}

func NewAPIKeyHandler(logger logrus.FieldLogger, apiKeyService APIKeyService) APIKeyHandler {
	return APIKeyHandler{
		logger:        logger,
		apiKeyService: apiKeyService,
	}
}

// apiKey represents a key fetched with a service query.
type apiKey struct {
	ID         uint       `json:"id"`
	Name       string     `json:"name"`
	Role       model.Role `json:"role"`
	LastSeenAt *time.Time `json:"lastSeenAt,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}

// generatedAPIKey represents newly generated API key with the JWT token.
type generatedAPIKey struct {
	ID        uint       `json:"id"`
	Name      string     `json:"name"`
	Role      model.Role `json:"role"`
	Key       string     `json:"key"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

type createAPIKeyRequest struct {
	Name       string     `json:"name"`
	Role       model.Role `json:"role"`
	TTLSeconds int64      `json:"ttlSeconds,omitempty"`
}

func apiKeyFromModel(m model.APIKey) apiKey {
	return apiKey{
		ID:         m.ID,
		Name:       m.Name,
		Role:       m.Role,
		ExpiresAt:  m.ExpiresAt,
		LastSeenAt: m.LastSeenAt,
		CreatedAt:  m.CreatedAt,
	}
}

func generatedAPIKeyFromModel(m model.APIKey) generatedAPIKey {
	return generatedAPIKey{
		ID:        m.ID,
		Name:      m.Name,
		Role:      m.Role,
		ExpiresAt: m.ExpiresAt,
		CreatedAt: m.CreatedAt,
	}
}

func (h APIKeyHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		HandleError(w, r, h.logger, JSONError{Err: err})
		return
	}
	params := model.CreateAPIKeyParams{
		Name: req.Name,
		Role: req.Role,
	}
	if req.TTLSeconds > 0 {
		expiresAt := time.Now().Add(time.Duration(req.TTLSeconds) * time.Second)
		params.ExpiresAt = &expiresAt
	}
	ak, secret, err := h.apiKeyService.CreateAPIKey(r.Context(), params)
	if err != nil {
		HandleError(w, r, h.logger, err)
		return
	}
	k := generatedAPIKeyFromModel(ak)
	k.Key = secret
	w.WriteHeader(http.StatusCreated)
	MustJSON(w, k)
}

func (h APIKeyHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	u, err := h.apiKeyService.GetAllAPIKeys(r.Context())
	if err != nil {
		HandleError(w, r, h.logger, err)
		return
	}
	apiKeys := make([]apiKey, len(u))
	for i := range u {
		apiKeys[i] = apiKeyFromModel(u[i])
	}
	MustJSON(w, apiKeys)
}

func (h APIKeyHandler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		HandleError(w, r, h.logger, err)
		return
	}
	if err = h.apiKeyService.DeleteAPIKeyByID(r.Context(), id); err != nil {
		HandleError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
