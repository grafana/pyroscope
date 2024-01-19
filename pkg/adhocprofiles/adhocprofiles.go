package adhocprofiles

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"sync"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/tenant"
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/adhocprofiles/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/pkg/errors"
)

type AdHocProfiles struct {
	services.Service
	logger      log.Logger
	bucket      objstore.Bucket
	buckets     map[string]objstore.Bucket
	bucketsLock sync.RWMutex
}

type AdHocProfile struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func NewAdHocProfiles(bucket objstore.Bucket, logger log.Logger) *AdHocProfiles {
	a := &AdHocProfiles{
		logger:  logger,
		bucket:  bucket,
		buckets: make(map[string]objstore.Bucket),
	}
	a.Service = services.NewBasicService(nil, a.running, nil)
	return a
}

func (a *AdHocProfiles) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (a *AdHocProfiles) Upload(ctx context.Context, c *connect.Request[v1.AdHocProfilesUploadRequest]) (*connect.Response[v1.AdHocProfilesGetResponse], error) {
	bucket, err := a.getBucket(ctx)
	if err != nil {
		return nil, err
	}
	adHocProfile := AdHocProfile{
		Name: c.Msg.Name,
		Data: c.Msg.Profile,
	}
	dataToStore, err := json.Marshal(adHocProfile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to upload profile")
	}

	// validate size
	// check if json, then upload directly

	flamegraph, err := parse(c.Msg.Profile, c.Msg.Name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse profile")
	}

	id := uuid.New()
	err = bucket.Upload(ctx, id.String(), bytes.NewReader(dataToStore))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to upload profile")
	}

	return connect.NewResponse(&v1.AdHocProfilesGetResponse{
		Flamegraph: flamegraph,
		Id:         id.String(),
	}), nil
}

func (a *AdHocProfiles) Get(ctx context.Context, c *connect.Request[v1.AdHocProfilesGetRequest]) (*connect.Response[v1.AdHocProfilesGetResponse], error) {
	bucket, err := a.getBucket(ctx)
	if err != nil {
		return nil, err
	}

	reader, err := bucket.Get(ctx, c.Msg.GetId())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get profile")
	}
	adHocProfileBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	var adHocProfile AdHocProfile
	err = json.Unmarshal(adHocProfileBytes, &adHocProfile)
	if err != nil {
		return nil, err
	}

	flamegraph, err := parse(adHocProfile.Data, adHocProfile.Name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse profile")
	}

	return connect.NewResponse(&v1.AdHocProfilesGetResponse{
		Id:         c.Msg.Id,
		Name:       adHocProfile.Name,
		Flamegraph: flamegraph,
	}), nil
}

func (a *AdHocProfiles) List(ctx context.Context, c *connect.Request[v1.AdHocProfilesListRequest]) (*connect.Response[v1.AdHocProfilesListResponse], error) {
	bucket, err := a.getBucket(ctx)
	if err != nil {
		return nil, err
	}
	profiles := make([]*v1.AdHocProfilesProfileMeta, 0)
	err = bucket.Iter(ctx, "", func(s string) error {
		profileReader, err := bucket.Get(ctx, s)
		if err != nil {
			return err
		}
		adHocProfileBytes, err := io.ReadAll(profileReader)
		if err != nil {
			return err
		}
		var adHocProfile AdHocProfile
		err = json.Unmarshal(adHocProfileBytes, &adHocProfile)
		if err != nil {
			return err
		}
		profiles = append(profiles, &v1.AdHocProfilesProfileMeta{
			Id:   s,
			Name: adHocProfile.Name,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.AdHocProfilesListResponse{Profiles: profiles}), nil
}

func (a *AdHocProfiles) getBucket(ctx context.Context) (objstore.Bucket, error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	a.bucketsLock.RLock()
	bucket, ok := a.buckets[tenantID]
	if !ok {
		a.bucketsLock.RUnlock()
		a.bucketsLock.Lock()
		bucket = objstore.NewPrefixedBucket(a.bucket, tenantID+"/adhoc")
		a.buckets[tenantID] = bucket
		a.bucketsLock.Unlock()
	} else {
		a.bucketsLock.RUnlock()
	}
	return bucket, nil
}

func parse(data string, name string) (*typesv1.FlameGraph, error) {
	base64decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to upload profile")
	}

	profiles, err := PprofToProfile(base64decoded, name, 65536)
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, errors.Wrapf(err, "no profiles found after parsing")
	}

	profile := profiles[0]
	levels := make([]*typesv1.Level, len(profile.Flamebearer.Levels))
	for i, level := range profile.Flamebearer.Levels {
		levelSlice := make([]int64, len(level))
		for j, v := range level {
			levelSlice[j] = int64(v)
		}
		levels[i] = &typesv1.Level{Values: levelSlice}
	}
	total := int64(0)
	if len(levels) > 0 {
		total = levels[0].Values[1]
	}

	return &typesv1.FlameGraph{
		Names:   profile.Flamebearer.Names,
		Levels:  levels,
		Total:   total,
		MaxSelf: int64(profile.Flamebearer.MaxSelf),
	}, nil
}
