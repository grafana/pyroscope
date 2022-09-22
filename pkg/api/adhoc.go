package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer/convert"
)

//go:generate mockgen -destination mocks/adhock.go -package mocks . AdhocService

type AdhocService interface {
	// GetProfileByID retrieves profile with the given ID.
	GetProfileByID(ctx context.Context, id string) (model.AdhocProfile, error)
	// GetAllProfiles lists all the known profiles.
	GetAllProfiles(context.Context) ([]model.AdhocProfile, error)
	// GetProfileDiffByID retrieves two profiles identified by their IDs and builds the profile diff.
	GetProfileDiffByID(context.Context, model.GetAdhocProfileDiffByIDParams) (model.AdhocProfile, error)
	// CreateProfile stores the profile provided and returns the entity created.
	CreateProfile(context.Context, model.CreateAdhocProfileParams) (model.AdhocProfile, error)
	// BuildProfileDiff takes two profiles and creates the difference flamegraph.
	// Implementation details: the result is not stored and never requested.
	BuildProfileDiff(context.Context, model.BuildAdhocProfileDiffParams) (model.AdhocProfile, error)
}

type AdhocHandler struct {
	adhocService AdhocService
	httpUtils    httputils.Utils
	maxBodySize  int64
	maxNodes     int
}

func NewAdhocHandler(adhocService AdhocService, httpUtils httputils.Utils) AdhocHandler {
	return AdhocHandler{
		adhocService: adhocService,
		httpUtils:    httpUtils,
		maxBodySize:  5 << 20, // 5M
	}
}

type adhocProfile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type buildProfileDiffRequest struct {
	Base *flamebearer.FlamebearerProfile `json:"base"`
	Diff *flamebearer.FlamebearerProfile `json:"diff"`
}

type adhocUploadRequest struct {
	Filename string               `json:"filename"`
	Profile  []byte               `json:"profile"`
	Type     string               `json:"type"`
	TypeData adhocProfileTypeData `json:"fileTypeData"`
}

type adhocProfileTypeData struct {
	SpyName string `json:"spyName"`
	Units   string `json:"units"`
}

type adhocUploadResponse struct {
	ID          string                          `json:"id"`
	Flamebearer *flamebearer.FlamebearerProfile `json:"flamebearer"`
}

func flamebearerFileFromAdhocRequest(req adhocUploadRequest) convert.ProfileFile {
	return convert.ProfileFile{
		Name:    req.Filename,
		Profile: req.Profile,
		Type:    convert.ProfileFileType(req.Type),
		TypeData: convert.ProfileFileTypeData{
			SpyName: req.TypeData.SpyName,
			Units:   metadata.Units(req.TypeData.Units),
		},
	}
}

func adhocProfileFromModel(m model.AdhocProfile) adhocProfile {
	return adhocProfile{
		ID:        m.ID,
		Name:      m.Name,
		UpdatedAt: m.UpdatedAt,
	}
}

func (h AdhocHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	p, err := h.adhocService.GetProfileByID(r.Context(), id)
	if err != nil {
		h.httpUtils.HandleError(r, w, err)
		return
	}
	h.httpUtils.MustJSON(r, w, p.Profile)
}

func (h AdhocHandler) ListProfiles(w http.ResponseWriter, r *http.Request) {
	p, err := h.adhocService.GetAllProfiles(r.Context())
	if err != nil {
		h.httpUtils.HandleError(r, w, err)
		return
	}
	profiles := make([]adhocProfile, len(p))
	for i := range p {
		profiles[i] = adhocProfileFromModel(p[i])
	}
	h.httpUtils.MustJSON(r, w, profiles)
}

func (h AdhocHandler) GetProfileDiff(w http.ResponseWriter, r *http.Request) {
	p, err := h.adhocService.GetProfileDiffByID(r.Context(), model.GetAdhocProfileDiffByIDParams{
		BaseID: mux.Vars(r)["left"],
		DiffID: mux.Vars(r)["right"],
	})
	if err != nil {
		h.httpUtils.HandleError(r, w, err)
		return
	}
	h.httpUtils.MustJSON(r, w, p.Profile)
}

func (h AdhocHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
	var req adhocUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpUtils.HandleError(r, w, httputils.JSONError{Err: err})
		return
	}
	params := model.CreateAdhocProfileParams{
		Profile: flamebearerFileFromAdhocRequest(req),
	}
	p, err := h.adhocService.CreateProfile(r.Context(), params)
	if err != nil {
		h.httpUtils.HandleError(r, w, err)
		return
	}
	h.httpUtils.MustJSON(r, w, adhocUploadResponse{
		ID:          p.ID,
		Flamebearer: p.Profile,
	})
}

func (h AdhocHandler) UploadDiff(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
	var req buildProfileDiffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpUtils.HandleError(r, w, httputils.JSONError{Err: err})
		return
	}
	params := model.BuildAdhocProfileDiffParams{
		Diff: req.Diff,
		Base: req.Base,
	}
	diff, err := h.adhocService.BuildProfileDiff(r.Context(), params)
	if err != nil {
		h.httpUtils.HandleError(r, w, err)
		return
	}
	h.httpUtils.MustJSON(r, w, diff.Profile)
}
