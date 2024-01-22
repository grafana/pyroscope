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
	"github.com/grafana/pyroscope/pkg/frontend"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
	"github.com/grafana/pyroscope/pkg/validation"
	"github.com/pkg/errors"
)

type AdHocProfiles struct {
	services.Service
	logger      log.Logger
	limits      frontend.Limits
	bucket      objstore.Bucket
	buckets     map[string]objstore.Bucket
	bucketsLock sync.RWMutex
}

type AdHocProfile struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func NewAdHocProfiles(bucket objstore.Bucket, logger log.Logger, limits frontend.Limits) *AdHocProfiles {
	a := &AdHocProfiles{
		logger:  logger,
		bucket:  bucket,
		buckets: make(map[string]objstore.Bucket),
		limits:  limits,
	}
	a.Service = services.NewBasicService(nil, a.running, nil)
	return a
}

func (a *AdHocProfiles) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (a *AdHocProfiles) Upload(ctx context.Context, c *connect.Request[v1.AdHocProfilesUploadRequest]) (*connect.Response[v1.AdHocProfilesUploadResponse], error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	bucket, err := a.getBucket(tenantID)
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

	maxNodes, err := validation.ValidateMaxNodes(a.limits, []string{tenantID}, c.Msg.GetMaxNodes())
	if err != nil {
		return nil, errors.Wrapf(err, "could not determine max nodes")
	}

	profile, sampleTypes, err := parse(c.Msg.Profile, nil, maxNodes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse profile")
	}

	id := uuid.New()
	err = bucket.Upload(ctx, id.String(), bytes.NewReader(dataToStore))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to upload profile")
	}

	jsonProfile, err := json.Marshal(profile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse profile")
	}

	return connect.NewResponse(&v1.AdHocProfilesUploadResponse{
		Id:                 id.String(),
		FlamebearerProfile: string(jsonProfile),
		SampleTypes:        sampleTypes,
	}), nil
}

func (a *AdHocProfiles) Get(ctx context.Context, c *connect.Request[v1.AdHocProfilesGetRequest]) (*connect.Response[v1.AdHocProfilesGetResponse], error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	bucket, err := a.getBucket(tenantID)
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

	maxNodes, err := validation.ValidateMaxNodes(a.limits, []string{tenantID}, c.Msg.GetMaxNodes())
	if err != nil {
		return nil, errors.Wrapf(err, "could not determine max nodes")
	}

	profile, sampleTypes, err := parse(adHocProfile.Data, c.Msg.SampleType, maxNodes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse profile")
	}

	sampleType := ""
	if c.Msg.SampleType != nil {
		sampleType = *c.Msg.SampleType
	}

	jsonProfile, err := json.Marshal(profile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse profile")
	}

	return connect.NewResponse(&v1.AdHocProfilesGetResponse{
		Id:                 c.Msg.Id,
		Name:               adHocProfile.Name,
		FlamebearerProfile: string(jsonProfile),
		SampleType:         sampleType,
		SampleTypes:        sampleTypes,
	}), nil
}

func (a *AdHocProfiles) List(ctx context.Context, c *connect.Request[v1.AdHocProfilesListRequest]) (*connect.Response[v1.AdHocProfilesListResponse], error) {
	bucket, err := a.getBucketFromContext(ctx)
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

func (a *AdHocProfiles) getBucketFromContext(ctx context.Context) (objstore.Bucket, error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return a.getBucket(tenantID)
}

func (a *AdHocProfiles) getBucket(tenantID string) (objstore.Bucket, error) {
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

func parse(data string, sampleType *string, maxNodes int64) (fg *flamebearer.FlamebearerProfile, sampleTypes []string, err error) {
	base64decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to upload profile")
	}

	profiles, err := PprofToProfile(base64decoded, int(maxNodes))
	if err != nil {
		return nil, nil, err
	}
	if len(profiles) == 0 {
		return nil, nil, errors.Wrapf(err, "no profiles found after parsing")
	}

	sampleTypes = make([]string, 0)
	sampleTypeIndex := -1
	for i, p := range profiles {
		sampleTypes = append(sampleTypes, p.Metadata.Name)
		if sampleType != nil && p.Metadata.Name == *sampleType {
			sampleTypeIndex = i
		}
	}

	var profile *flamebearer.FlamebearerProfile
	if sampleTypeIndex >= 0 {
		profile = profiles[sampleTypeIndex]
	} else {
		profile = profiles[0]
	}

	return profile, sampleTypes, nil
}
