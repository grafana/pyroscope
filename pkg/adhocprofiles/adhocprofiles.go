package adhocprofiles

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/tenant"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"

	v1 "github.com/grafana/pyroscope/api/gen/proto/go/adhocprofiles/v1"
	"github.com/grafana/pyroscope/pkg/frontend"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer/convert"
	"github.com/grafana/pyroscope/pkg/validation"
)

type AdHocProfiles struct {
	services.Service

	logger log.Logger
	limits frontend.Limits
	bucket objstore.Bucket
}

type AdHocProfile struct {
	Name       string    `json:"name"`
	Data       string    `json:"data"`
	UploadedAt time.Time `json:"uploadedAt"`
}

func NewAdHocProfiles(bucket objstore.Bucket, logger log.Logger, limits frontend.Limits) *AdHocProfiles {
	a := &AdHocProfiles{
		logger: logger,
		bucket: bucket,
		limits: limits,
	}
	a.Service = services.NewBasicService(nil, a.running, nil)
	return a
}

func (a *AdHocProfiles) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (a *AdHocProfiles) Upload(ctx context.Context, c *connect.Request[v1.AdHocProfilesUploadRequest]) (*connect.Response[v1.AdHocProfilesGetResponse], error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	adHocProfile := AdHocProfile{
		Name:       c.Msg.Name,
		Data:       c.Msg.Profile,
		UploadedAt: time.Now().UTC(),
	}

	// TODO: Add per-tenant upload limits (number of files, total size, etc.)

	maxNodes, err := validation.ValidateMaxNodes(a.limits, []string{tenantID}, c.Msg.GetMaxNodes())
	if err != nil {
		return nil, errors.Wrapf(err, "could not determine max nodes")
	}

	profile, profileTypes, err := parse(&adHocProfile, nil, maxNodes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse profile")
	}

	bucket := a.getBucket(tenantID)

	uid := ulid.MustNew(ulid.Timestamp(adHocProfile.UploadedAt), rand.Reader)
	id := strings.Join([]string{uid.String(), adHocProfile.Name}, "-")

	dataToStore, err := json.Marshal(adHocProfile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to upload profile")
	}

	err = bucket.Upload(ctx, id, bytes.NewReader(dataToStore))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to upload profile")
	}

	jsonProfile, err := json.Marshal(profile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse profile")
	}

	return connect.NewResponse(&v1.AdHocProfilesGetResponse{
		Id:                 id,
		Name:               adHocProfile.Name,
		UploadedAt:         adHocProfile.UploadedAt.UnixMilli(),
		FlamebearerProfile: string(jsonProfile),
		ProfileType:        profile.Metadata.Name,
		ProfileTypes:       profileTypes,
	}), nil
}

func (a *AdHocProfiles) Get(ctx context.Context, c *connect.Request[v1.AdHocProfilesGetRequest]) (*connect.Response[v1.AdHocProfilesGetResponse], error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	bucket := a.getBucket(tenantID)

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

	profile, profileTypes, err := parse(&adHocProfile, c.Msg.ProfileType, maxNodes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse profile")
	}

	jsonProfile, err := json.Marshal(profile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse profile")
	}

	return connect.NewResponse(&v1.AdHocProfilesGetResponse{
		Id:                 c.Msg.Id,
		Name:               adHocProfile.Name,
		UploadedAt:         adHocProfile.UploadedAt.UnixMilli(),
		FlamebearerProfile: string(jsonProfile),
		ProfileType:        profile.Metadata.Name,
		ProfileTypes:       profileTypes,
	}), nil
}

func (a *AdHocProfiles) List(ctx context.Context, c *connect.Request[v1.AdHocProfilesListRequest]) (*connect.Response[v1.AdHocProfilesListResponse], error) {
	bucket, err := a.getBucketFromContext(ctx)
	if err != nil {
		return nil, err
	}

	profiles := make([]*v1.AdHocProfilesProfileMetadata, 0)
	err = bucket.Iter(ctx, "", func(s string) error {
		separatorIndex := strings.IndexRune(s, '-')
		id, err := ulid.Parse(s[0:separatorIndex])
		if err != nil {
			level.Warn(a.logger).Log("msg", "cannot parse ad hoc profile", "key", s, "err", err)
			return nil
		}
		name := s[separatorIndex+1:]
		profiles = append(profiles, &v1.AdHocProfilesProfileMetadata{
			Id:         s,
			Name:       name,
			UploadedAt: int64(id.Time()),
		})
		return nil
	})
	cmp := func(a, b *v1.AdHocProfilesProfileMetadata) int {
		if a.UploadedAt < b.UploadedAt {
			return 1
		}
		if a.UploadedAt > b.UploadedAt {
			return -1
		}
		return 0
	}
	slices.SortFunc(profiles, cmp)
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
	return a.getBucket(tenantID), nil
}

func (a *AdHocProfiles) getBucket(tenantID string) objstore.Bucket {
	return objstore.NewPrefixedBucket(a.bucket, tenantID+"/adhoc")
}

func parse(p *AdHocProfile, profileType *string, maxNodes int64) (fg *flamebearer.FlamebearerProfile, profileTypes []string, err error) {
	base64decoded, err := base64.StdEncoding.DecodeString(p.Data)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to upload profile")
	}

	f := convert.ProfileFile{
		Name:     p.Name,
		TypeData: convert.ProfileFileTypeData{},
		Data:     base64decoded,
	}

	profiles, err := convert.FlamebearerFromFile(f, int(maxNodes))
	if err != nil {
		return nil, nil, err
	}
	if len(profiles) == 0 {
		return nil, nil, errors.Wrapf(err, "no profiles found after parsing")
	}

	profileTypes = make([]string, 0)
	profileTypeIndex := -1
	for i, p := range profiles {
		profileTypes = append(profileTypes, p.Metadata.Name)
		if profileType != nil && p.Metadata.Name == *profileType {
			profileTypeIndex = i
		}
	}

	var profile *flamebearer.FlamebearerProfile
	if profileTypeIndex >= 0 {
		profile = profiles[profileTypeIndex]
	} else {
		profile = profiles[0]
	}

	return profile, profileTypes, nil
}
