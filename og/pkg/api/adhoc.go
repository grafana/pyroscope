package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	GetProfileByID(ctx context.Context, id string) (*flamebearer.FlamebearerProfile, error)
	// GetAllProfiles lists all the known profiles.
	GetAllProfiles(context.Context) ([]model.AdhocProfile, error)
	// GetProfileDiffByID retrieves two profiles identified by their IDs and builds the profile diff.
	GetProfileDiffByID(context.Context, model.GetAdhocProfileDiffByIDParams) (*flamebearer.FlamebearerProfile, error)
	// UploadProfile stores the profile provided and returns the entity created.
	UploadProfile(context.Context, model.UploadAdhocProfileParams) (p *flamebearer.FlamebearerProfile, id string, err error)
}

type AdhocHandler struct {
	adhocService AdhocService
	httpUtils    httputils.Utils
	maxBodySize  int64
	maxNodes     int
}

func NewAdhocHandler(adhocService AdhocService, httpUtils httputils.Utils, maxBodySize int64) AdhocHandler {
	return AdhocHandler{
		adhocService: adhocService,
		httpUtils:    httpUtils,
		maxBodySize:  maxBodySize,
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
		Name: req.Filename,
		Data: req.Profile,
		Type: convert.ProfileFileType(req.Type),
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
	h.httpUtils.MustJSON(r, w, p)
}

func (h AdhocHandler) GetProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.adhocService.GetAllProfiles(r.Context())
	if err != nil {
		h.httpUtils.HandleError(r, w, err)
		return
	}
	resp := make(map[string]adhocProfile, len(profiles))
	for _, p := range profiles {
		resp[p.ID] = adhocProfileFromModel(p)
	}
	h.httpUtils.MustJSON(r, w, resp)
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
	h.httpUtils.MustJSON(r, w, p)
}

func (h AdhocHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if h.maxBodySize > 0 {
		r.Body = &MaxBytesReader{http.MaxBytesReader(w, r.Body, h.maxBodySize)}
	}
	var req adhocUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.httpUtils.HandleError(r, w, httputils.JSONError{Err: err})
		return
	}
	params := model.UploadAdhocProfileParams{
		Profile: flamebearerFileFromAdhocRequest(req),
	}
	p, id, err := h.adhocService.UploadProfile(r.Context(), params)
	if err != nil {
		h.httpUtils.HandleError(r, w, err)
		return
	}
	h.httpUtils.MustJSON(r, w, adhocUploadResponse{
		ID:          id,
		Flamebearer: p,
	})
}

type MaxBytesReader struct {
	r io.ReadCloser
}

func (m MaxBytesReader) Read(p []byte) (n int, err error) {
	n, err = m.r.Read(p)
	if err != nil {
		targetErr := &http.MaxBytesError{}
		if errors.As(err, &targetErr) {
			err = fmt.Errorf("profile too large, max size is %d bytes", targetErr.Limit)
		}
	}
	return n, err
}

func (m *MaxBytesReader) Close() error {
	return m.r.Close()
}
