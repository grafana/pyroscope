package server

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
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

	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
)

const maxBodySize = 5242880 // 5MB

type profile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type server struct {
	log      logrus.FieldLogger
	dataDir  string
	maxNodes int
	enabled  bool
	profiles map[string]profile
	mtx      sync.RWMutex
}

type Server interface {
	AddRoutes(router *mux.Router) http.HandlerFunc
}

func New(log logrus.FieldLogger, dataDir string, maxNodes int, enabled bool) Server {
	return &server{
		log:      log,
		dataDir:  dataDir,
		maxNodes: maxNodes,
		enabled:  enabled,
		profiles: make(map[string]profile),
	}
}

func (s *server) AddRoutes(r *mux.Router) http.HandlerFunc {
	if s.enabled {
		r.HandleFunc("/v1/profiles", s.Profiles)
		r.HandleFunc("/v1/profile/{id:[0-9a-f]+}", s.Profile)
		r.HandleFunc("/v1/diff/{left:[0-9a-f]+}/{right:[0-9a-f]+}", s.Diff)
		r.HandleFunc("/v1/upload/", s.Upload)
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
	if err := os.MkdirAll(s.dataDir, os.ModeDir|os.ModePerm); err != nil {
		s.log.WithError(err).Errorf("Unable to create data directory")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	profiles := make(map[string]profile, 0)
	err := filepath.WalkDir(s.dataDir, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() && path != s.dataDir {
			return fs.SkipDir
		}
		if e.Type().IsRegular() {
			id := fmt.Sprintf("%x", sha256.Sum256([]byte(e.Name())))
			if p, ok := profiles[id]; ok {
				return fmt.Errorf("a hash collision detected between %s and %s, please report it", e.Name(), p.Name)
			}
			info, err := e.Info()
			if err != nil {
				return fmt.Errorf("unable to retrieve entry information: %w", err)
			}
			profiles[id] = profile{ID: id, Name: e.Name(), UpdatedAt: info.ModTime()}
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
	fb, err := s.convert(p)
	if err != nil {
		s.log.WithError(err).Error("Unable to process profile")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(*fb); err != nil {
		s.log.WithError(err).Error("Unable to marshal profile")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// Diff retrieves two different local files identified by their IDs and builds a profile diff.
func (s *server) Diff(w http.ResponseWriter, r *http.Request) {
	lid := mux.Vars(r)["left"]
	rid := mux.Vars(r)["right"]
	s.mtx.RLock()
	lp, lok := s.profiles[lid]
	rp, rok := s.profiles[rid]
	s.mtx.RUnlock()
	if !lok {
		s.log.WithField("id", lid).Warning("Profile does not exist")
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	if !rok {
		s.log.WithField("id", rid).Warning("Profile does not exist")
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	lfb, err := s.convert(lp)
	if err != nil {
		s.log.WithError(err).Error("Unable to process left profile")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rfb, err := s.convert(rp)
	if err != nil {
		s.log.WithError(err).Error("Unable to process right profile")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fb, err := DiffV1(lfb, rfb, s.maxNodes)
	if err != nil {
		s.log.WithError(err).Error("Unable to generate a diff profile")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(fb); err != nil {
		s.log.WithError(err).Error("Unable to encode the profile diff")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// Upload a profile in any of the supported formats and convert it to pyroscope internal format.
func (s *server) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	var m Model
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		msg := "Unable to decode the body of the request. " +
			"A JSON body with a base64 encoded `profile`, " +
			"an optional string `filename`, and an optional string `type` is expected."
		s.log.WithError(err).Error(msg)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	r.Body.Close()
	converter, err := m.Converter()
	if err != nil {
		msg := "Unable to detect the profile format based on the type, " +
			"the filename and its contents. Currently supported formats are " +
			"pprof's protobuf profile, pyroscope's JSON format and collapsed formats."
		s.log.WithError(err).Error(msg)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if converter == nil {
		msg := "Unable to detect the profile format based on the type, " +
			"the filename and its contents. Currently supported formats are " +
			"pprof's protobuf profile, pyroscope's JSON format and collapsed formats."
		s.log.WithError(err).Error(msg)
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		return
	}
	fb, err := converter(m.Profile, m.Filename, s.maxNodes)
	if err != nil {
		msg := "Unable to convert the profile to our internal format. " +
			"The profile was detected as " + ConverterToFormat(converter)
		s.log.WithError(err).Error(msg)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(fb); err != nil {
		s.log.WithError(err).Error("Unable to encode the response")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) convert(p profile) (*flamebearer.FlamebearerProfile, error) {
	fname := filepath.Join(s.dataDir, p.Name)
	f, err := os.Open(fname)
	if err != nil {
		return nil, fmt.Errorf("unable to open profile: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("unable to read profile: %w", err)
	}
	m := Model{
		Filename: fname,
		Profile:  b,
	}
	converter, err := m.Converter()
	if err != nil {
		return nil, fmt.Errorf("unable to handle the profile format: %w", err)
	}
	if converter == nil {
		return nil, errors.New("unsupported profile format")
	}
	return converter(b, p.Name, s.maxNodes)
}
