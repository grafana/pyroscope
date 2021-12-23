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
	enabled  bool
	profiles map[string]profile
	mtx      sync.RWMutex
}

type Server interface {
	AddRoutes(router *mux.Router) http.HandlerFunc
}

func New(log logrus.FieldLogger, enabled bool) Server {
	return &server{log: log, enabled: enabled, profiles: make(map[string]profile)}
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
	dataDir, err := util.EnsureDataDirectory()
	if err != nil {
		s.log.WithError(err).Errorf("Unable to create data directory")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	profiles := make(map[string]profile, 0)
	err = filepath.Walk(dataDir, func(path string, info fs.FileInfo, err error) error {
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
// TODO(abeaumont): Support other formats, only native JSON is supported for now.
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
	dataDir, err := util.EnsureDataDirectory()
	if err != nil {
		s.log.WithError(err).Errorf("Unable to create data directory")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	f, err := os.Open(filepath.Join(dataDir, p.Name))
	if err != nil {
		s.log.WithError(err).Error("Unable to open profile")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	// Validate the format
	b, err := io.ReadAll(f)
	if err != nil {
		s.log.WithError(err).Error("Unable to read profile")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var fb flamebearer.FlamebearerProfile
	if err := json.Unmarshal(b, &fb); err != nil {
		s.log.WithError(err).Error("Invalid file format")
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
