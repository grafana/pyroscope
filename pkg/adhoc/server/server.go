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
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/adhoc/writer"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
)

const maxBodySize = 5242880 // 5MB

type profile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type server struct {
	log                logrus.FieldLogger
	dataDir            string
	adhocDataDirWriter *writer.AdhocDataDirWriter
	maxNodes           int
	enabled            bool
	profiles           map[string]profile
	mtx                sync.RWMutex
}

type Server interface {
	AddRoutes(router *mux.Router) http.HandlerFunc
}

func New(log logrus.FieldLogger, dataDir string, maxNodes int, enabled bool) Server {
	return &server{
		log:                log,
		adhocDataDirWriter: writer.NewAdhocDataDirWriter(dataDir),
		dataDir:            dataDir,
		maxNodes:           maxNodes,
		enabled:            enabled,
		profiles:           make(map[string]profile),
	}
}

func (s *server) AddRoutes(r *mux.Router) http.HandlerFunc {
	if s.enabled {
		r.HandleFunc("/v1/profiles", s.Profiles)
		r.HandleFunc("/v1/profile/{id:[0-9a-f]+}", s.Profile)
		r.HandleFunc("/v1/diff/{left:[0-9a-f]+}/{right:[0-9a-f]+}", s.Diff)
		r.HandleFunc("/v1/upload", s.Upload)
		r.HandleFunc("/v1/upload-diff/", s.UploadDiff)
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
			id := s.generateHash(e.Name())
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

	// Try to get a name for the profile.
	var name string
	for _, n := range []string{lfb.Metadata.Name, rfb.Metadata.Name, lp.Name, rp.Name} {
		if n != "" {
			name = n
			break
		}
	}
	fb, err := DiffV1(name, lfb, rfb, s.maxNodes)
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

	err = s.adhocDataDirWriter.EnsureExists()
	if err != nil {
		msg := "Unable to create adhoc's DataPath"
		s.log.WithError(err).Error(msg)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	now := time.Now()
	// Remove extension, since we will store a json file
	filename := strings.TrimSuffix(m.Filename, filepath.Ext(m.Filename))
	// TODO(eh-am): maybe we should use whatever the user has sent us?
	filename = fmt.Sprintf("%s-%s.json", filename, now.Format("2006-01-02-15-04-05"))

	_, err = s.adhocDataDirWriter.Write(filename, *fb)
	if err != nil {
		msg := "Unable to write profile to adhoc's DataPath"
		s.log.WithError(err).Error(msg)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	type response struct {
		Flamebearer *flamebearer.FlamebearerProfile `json:"flamebearer"`
		ID          string                          `json:"id"`
	}
	res := response{Flamebearer: fb, ID: s.generateHash(filename)}
	if err := json.NewEncoder(w).Encode(res); err != nil {
		s.log.WithError(err).Error("Unable to encode the response")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// Upload two single profiles in native JSON format and convert to a diff profile
func (s *server) UploadDiff(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	var m diffModel
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		msg := "Unable to decode the body of the request. " +
			"A JSON body with a a `base` and a `diff` profile field is expected."
		s.log.WithError(err).Error(msg)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	r.Body.Close()

	fb, err := DiffV1("", &m.Base, &m.Diff, s.maxNodes)
	if err != nil {
		s.log.WithError(err).Error("Unable to generate a diff profile")
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

func (*server) generateHash(name string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(name)))
}
