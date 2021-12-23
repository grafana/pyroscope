package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/adhoc/util"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
)

type profile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type server struct {
	log      logrus.FieldLogger
	maxNodes int
	enabled  bool
	profiles map[string]profile
	mtx      sync.RWMutex
}

type Server interface {
	AddRoutes(router *mux.Router) http.HandlerFunc
}

func New(log logrus.FieldLogger, maxNodes int, enabled bool) Server {
	return &server{
		log:      log,
		maxNodes: maxNodes,
		enabled:  enabled,
		profiles: make(map[string]profile),
	}
}

func (s *server) AddRoutes(r *mux.Router) http.HandlerFunc {
	if s.enabled {
		r.HandleFunc("/v1/profiles", s.Profiles)
		r.HandleFunc("/v1/profile/{id:[0-9a-f]+}", s.Profile)
		r.HandleFunc("/v1/diff/{id1[0-9a-f]+}/{id2[0-9a-f]+}", s.Diff)
	}
	return r.ServeHTTP
}

// Profiles retrieves the list of profiles for the local pyroscope data directory.
// The profiles are assigned a unique ID (hash based) which is then used for retrieval.
// This requires a bit of extra work to setup the IDs but prevents
// the clients from accesing the filesystem directly, removing that whole attack vector.
//
// The profiles are retrieved every time the endpoint is requested,
// which should be good enough as massive access to this auth endpoint is not expected.
func (s *server) Profiles(w http.ResponseWriter, _ *http.Request) {
	profiles := make(map[string]profile, 0)
	err := filepath.Walk(util.DataDirectory(), func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			id := fmt.Sprintf("%x", sha256.Sum256([]byte(info.Name())))
			if p, ok := profiles[id]; ok {
				return fmt.Errorf("A hash collision detected between %s and %s, please report it", info.Name(), p.Name)
			}
			profiles[id] = profile{ID: id, Name: info.Name(), UpdatedAt: info.ModTime()}
		}
		return nil
	})
	if err != nil {
		s.log.WithError(err).Error("Unable to retrieve the profile list")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mtx.Lock()
	s.profiles = profiles
	s.mtx.Unlock()
	w.Header().Set("Content-Type", "application/json")
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	if err := json.NewEncoder(w).Encode(s.profiles); err != nil {
		s.log.WithError(err).Error("Unable to encode the profile list")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// Profile retrieves a local file identified by its ID.
func (s *server) Profile(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	s.mtx.RLock()
	p, ok := s.profiles[id]
	s.mtx.RUnlock()
	if !ok {
		s.log.WithField("id", id).Warning("Profile does not exist")
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	fb, err := s.profile(p)
	if err != nil {
		s.log.WithError(err).Error("Unable to process profile")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	b, err := json.Marshal(*fb)
	if err != nil {
		s.log.WithError(err).Error("Unable to marshall profile")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(b); err != nil {
		s.log.WithError(err).Error("Error sending profile")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// TODO(abeaumont)
func (*server) Diff(_ http.ResponseWriter, _ *http.Request) {
}

type converterFn func(b []byte, name string, maxNodes int) (*flamebearer.FlamebearerProfile, error)

func (s *server) profile(p profile) (*flamebearer.FlamebearerProfile, error) {
	fname := filepath.Join(util.DataDirectory(), p.Name)
	ext := filepath.Ext(fname)
	var converter converterFn
	switch ext {
	case ".json":
		converter = jsonToProfile
	case ".pprof":
		converter = pprofToProfile
	case ".txt":
		converter = collapsedToProfile
	default:
		return nil, fmt.Errorf("unsupported file extension %s", ext)
	}
	f, err := os.Open(fname)
	if err != nil {
		return nil, fmt.Errorf("unable to open profile: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("unable to read profile: %w", err)
	}
	return converter(b, p.Name, s.maxNodes)
}
