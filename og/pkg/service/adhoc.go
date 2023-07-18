package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/adhoc/writer"
	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer/convert"
)

type AdhocService struct {
	adhocWriter *writer.AdhocDataDirWriter
	dataDir     string
	maxNodes    int

	m        sync.RWMutex
	profiles map[string]model.AdhocProfile
}

func NewAdhocService(maxNodes int, dataDir string) *AdhocService {
	return &AdhocService{
		adhocWriter: writer.NewAdhocDataDirWriter(dataDir),
		dataDir:     dataDir,
		maxNodes:    maxNodes,
		profiles:    make(map[string]model.AdhocProfile),
	}
}

func (svc *AdhocService) GetProfileByID(_ context.Context, id string) (*flamebearer.FlamebearerProfile, error) {
	svc.m.RLock()
	p, ok := svc.profiles[id]
	svc.m.RUnlock()
	if !ok {
		return nil, model.ErrAdhocProfileNotFound
	}
	fb, err := svc.loadProfile(p)
	if err != nil {
		return nil, fmt.Errorf("unable to process profile: %w", err)
	}
	return fb, nil
}

// GetAllProfiles retrieves the list of profiles for the local pyroscope data directory.
// The profiles are assigned a unique ID (hash based) which is then used for retrieval.
// This requires a bit of extra work to setup the IDs but prevents
// the clients from accesing the filesystem directly, removing that whole attack vector.
//
// The profiles are retrieved every time the endpoint is requested,
// which should be good enough as massive access to this auth endpoint is not expected.
func (svc *AdhocService) GetAllProfiles(_ context.Context) ([]model.AdhocProfile, error) {
	if err := os.MkdirAll(svc.dataDir, os.ModeDir|os.ModePerm); err != nil {
		return nil, fmt.Errorf("unable to create data directory: %w", err)
	}
	profiles := make(map[string]model.AdhocProfile, 0)
	err := filepath.WalkDir(svc.dataDir, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() && path != svc.dataDir {
			return fs.SkipDir
		}
		if e.Type().IsRegular() {
			id := svc.generateHash(e.Name())
			if p, ok := profiles[id]; ok {
				return fmt.Errorf("a hash collision detected between %s and %s, please report it", e.Name(), p.Name)
			}
			info, err := e.Info()
			if err != nil {
				return fmt.Errorf("unable to retrieve entry information: %w", err)
			}
			profiles[id] = model.AdhocProfile{ID: id, Name: e.Name(), UpdatedAt: info.ModTime()}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("retrieving the profile list: %w", err)
	}
	profilesCopy := make([]model.AdhocProfile, 0, len(svc.profiles))
	for _, p := range profiles {
		profilesCopy = append(profilesCopy, p)
	}
	svc.m.Lock()
	svc.profiles = profiles
	svc.m.Unlock()
	return profilesCopy, nil
}

func (svc *AdhocService) GetProfileDiffByID(_ context.Context, params model.GetAdhocProfileDiffByIDParams) (*flamebearer.FlamebearerProfile, error) {
	svc.m.RLock()
	bp, bok := svc.profiles[params.BaseID]
	dp, dok := svc.profiles[params.DiffID]
	svc.m.RUnlock()
	if !bok || !dok {
		return nil, model.ErrAdhocProfileNotFound
	}
	var err error
	bfb, err := svc.loadProfile(bp)
	if err != nil {
		return nil, fmt.Errorf("unable to process left profile: %w", err)
	}
	dfb, err := svc.loadProfile(dp)
	if err != nil {
		return nil, fmt.Errorf("unable to process right profile: %w", err)
	}
	// Try to get a name for the profile.
	var name string
	for _, n := range []string{bfb.Metadata.Name, dfb.Metadata.Name, bp.Name, dp.Name} {
		if n != "" {
			name = n
			break
		}
	}
	fb, err := flamebearer.Diff(name, bfb, dfb, svc.maxNodes)
	if err != nil {
		return nil, fmt.Errorf("unable to generate a diff profile: %w", err)
	}
	return &fb, nil
}

func (svc *AdhocService) UploadProfile(_ context.Context, params model.UploadAdhocProfileParams) (*flamebearer.FlamebearerProfile, string, error) {
	fb, err := convert.FlamebearerFromFile(params.Profile, svc.maxNodes)
	if err != nil {
		return nil, "", model.ValidationError{Err: err}
	}
	err = svc.adhocWriter.EnsureExists()
	if err != nil {
		return nil, "", fmt.Errorf("unable to create data directory: %w", err)
	}
	now := time.Now()
	// Remove extension, since we will store a json file
	filename := strings.TrimSuffix(params.Profile.Name, filepath.Ext(params.Profile.Name))
	// TODO(eh-am): maybe we should use whatever the user has sent us?
	// TODO(kolesnikovae): I agree that we should store the original
	//   user input, however, it's pretty problematic to change without
	//   violation of the backward compatibility.
	filename = fmt.Sprintf("%s-%s.json", filename, now.Format("2006-01-02-15-04-05"))
	if _, err = svc.adhocWriter.Write(filename, *fb); err != nil {
		return nil, "", fmt.Errorf("unable to write profile to the data directory: %w", err)
	}
	return fb, svc.generateHash(filename), nil
}

func (svc *AdhocService) loadProfile(p model.AdhocProfile) (*flamebearer.FlamebearerProfile, error) {
	fileName := filepath.Join(svc.dataDir, p.Name)
	f, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("unable to open profile: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("unable to read profile: %w", err)
	}
	return convert.FlamebearerFromFile(convert.ProfileFile{Name: fileName, Data: b}, svc.maxNodes)
}

func (*AdhocService) generateHash(name string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(name)))
}
