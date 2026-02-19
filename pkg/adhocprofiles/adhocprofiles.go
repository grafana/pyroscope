package adhocprofiles

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/tenant"
	"github.com/oklog/ulid/v2"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	v1 "github.com/grafana/pyroscope/api/gen/proto/go/adhocprofiles/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer/convert"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/validation"
)

type AdHocProfiles struct {
	services.Service

	logger log.Logger
	limits Limits
	bucket objstore.Bucket
}

type Limits interface {
	validation.FlameGraphLimits
	MaxProfileSizeBytes(tenantID string) int
}

type AdHocProfile struct {
	Name       string    `json:"name"`
	Data       string    `json:"data"`
	UploadedAt time.Time `json:"uploadedAt"`
}

func validRunes(r rune) bool {
	if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
		return true
	}
	return false
}

// check if the id is valid
func validID(id string) bool {
	if len(id) == 0 {
		return false
	}
	for _, r := range id {
		if !validRunes(r) {
			return false
		}
	}
	return true
}

// replaces invalid runes in the id with underscores
func replaceInvalidRunes(id string) string {
	return strings.Map(func(r rune) rune {
		if validRunes(r) {
			return r
		}
		return '_'
	}, id)
}

func NewAdHocProfiles(bucket objstore.Bucket, logger log.Logger, limits Limits) *AdHocProfiles {
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

	// replace runes outside of [a-zA-Z0-9_-.] with underscores
	adHocProfile.Name = replaceInvalidRunes(adHocProfile.Name)

	limits, err := a.newConvertLimits(tenantID, c.Msg.GetMaxNodes())
	if err != nil {
		return nil, err
	}

	// TODO: Add more per-tenant upload limits (number of files, total size, etc.)
	if limits.MaxProfileSizeBytes > 0 && len(adHocProfile.Data) > limits.MaxProfileSizeBytes {
		return nil, connect.NewError(connect.CodeInvalidArgument, validation.NewErrorf(validation.ProfileSizeLimit, "profile payload size exceeds limit of %s", humanize.Bytes(uint64(limits.MaxProfileSizeBytes))))
	}

	profile, profileTypes, err := parse(&adHocProfile, nil, limits)
	if err != nil {
		dsErr := new(pprof.ErrDecompressedSizeExceedsLimit)
		if errors.As(err, &dsErr) {
			return nil, connect.NewError(connect.CodeInvalidArgument, validation.NewErrorf(validation.ProfileSizeLimit, "uncompressed profile payload size exceeds limit of %s", humanize.Bytes(uint64(dsErr.Limit))))
		}
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

func (a *AdHocProfiles) newConvertLimits(tenantID string, msgMaxNodes int64) (convert.Limits, error) {
	maxNodes, err := validation.ValidateMaxNodes(a.limits, []string{tenantID}, msgMaxNodes)
	if err != nil {
		return convert.Limits{}, errors.Wrapf(err, "could not determine max nodes")
	}

	return convert.Limits{
		MaxNodes:            int(maxNodes),
		MaxProfileSizeBytes: a.limits.MaxProfileSizeBytes(tenantID),
	}, nil
}

func (a *AdHocProfiles) Get(ctx context.Context, c *connect.Request[v1.AdHocProfilesGetRequest]) (*connect.Response[v1.AdHocProfilesGetResponse], error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	bucket := a.getBucket(tenantID)

	id := c.Msg.GetId()
	if !validID(id) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id '%s' is invalid: can only contain [a-zA-Z0-9_-.]", id))
	}

	reader, err := bucket.Get(ctx, id)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get profile")
	}
	defer func() {
		_ = reader.Close()
	}()

	adHocProfileBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var adHocProfile AdHocProfile
	err = json.Unmarshal(adHocProfileBytes, &adHocProfile)
	if err != nil {
		return nil, err
	}

	limits, err := a.newConvertLimits(tenantID, c.Msg.GetMaxNodes())
	if err != nil {
		return nil, err
	}

	profile, profileTypes, err := parse(&adHocProfile, c.Msg.ProfileType, limits)
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
		// do not list elements with invalid ids
		if !validID(s) {
			return nil
		}

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

func (a *AdHocProfiles) Diff(ctx context.Context, c *connect.Request[v1.AdHocProfilesDiffRequest]) (*connect.Response[v1.AdHocProfilesDiffResponse], error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	bucket := a.getBucket(tenantID)

	leftID := c.Msg.GetLeftId()
	rightID := c.Msg.GetRightId()
	for _, id := range []string{leftID, rightID} {
		if !validID(id) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id '%s' is invalid: can only contain [a-zA-Z0-9_-.]", id))
		}
	}

	limits, err := a.newConvertLimits(tenantID, c.Msg.GetMaxNodes())
	if err != nil {
		return nil, err
	}

	var leftProfile, rightProfile AdHocProfile
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		p, err := fetchProfile(ctx, bucket, leftID)
		if err != nil {
			return fmt.Errorf("left profile: %w", err)
		}
		leftProfile = *p
		return nil
	})
	g.Go(func() error {
		p, err := fetchProfile(ctx, bucket, rightID)
		if err != nil {
			return fmt.Errorf("right profile: %w", err)
		}
		rightProfile = *p
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, errors.Wrap(err, "failed to fetch profiles")
	}

	leftFB, leftTypes, err := parse(&leftProfile, c.Msg.ProfileType, limits)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse left profile")
	}
	rightFB, rightTypes, err := parse(&rightProfile, c.Msg.ProfileType, limits)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse right profile")
	}

	// Compute intersection of profile types.
	rightSet := make(map[string]struct{}, len(rightTypes))
	for _, pt := range rightTypes {
		rightSet[pt] = struct{}{}
	}
	var commonTypes []string
	for _, pt := range leftTypes {
		if _, ok := rightSet[pt]; ok {
			commonTypes = append(commonTypes, pt)
		}
	}
	if len(commonTypes) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("no common profile types between left (%v) and right (%v)", leftTypes, rightTypes))
	}

	// Ensure both profiles have the same selected type to avoid comparing mismatched profile types.
	if leftFB.Metadata.Name != rightFB.Metadata.Name {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("profile type mismatch: left profile uses '%s', right profile uses '%s'; please specify a profile type from the common types: %v", leftFB.Metadata.Name, rightFB.Metadata.Name, commonTypes))
	}

	leftTree, err := flamebearerToModelTree(leftFB)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert left profile to tree")
	}
	rightTree, err := flamebearerToModelTree(rightFB)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert right profile to tree")
	}

	diff, err := model.NewFlamegraphDiff(leftTree, rightTree, int64(limits.MaxNodes))
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute diff")
	}

	profileType := &typesv1.ProfileType{
		Name:       leftFB.Metadata.Name,
		SampleType: leftFB.Metadata.Name,
		SampleUnit: string(leftFB.Metadata.Units),
	}

	diffFB := model.ExportDiffToFlamebearer(diff, profileType)
	jsonProfile, err := json.Marshal(diffFB)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal diff profile")
	}

	return connect.NewResponse(&v1.AdHocProfilesDiffResponse{
		ProfileTypes:       commonTypes,
		FlamebearerProfile: string(jsonProfile),
	}), nil
}

func fetchProfile(ctx context.Context, bucket objstore.Bucket, id string) (*AdHocProfile, error) {
	reader, err := bucket.Get(ctx, id)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get profile %s", id)
	}
	defer func() {
		_ = reader.Close()
	}()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var p AdHocProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// flamebearerToModelTree converts a FlamebearerProfile (format: "single") to a *model.Tree
// suitable for use with model.NewFlamegraphDiff.
func flamebearerToModelTree(fb *flamebearer.FlamebearerProfile) (*model.Tree, error) {
	ogTree, err := flamebearer.ProfileToTree(*fb)
	if err != nil {
		return nil, err
	}

	t := new(model.Tree)
	ogTree.IterateStacks(func(_ string, self uint64, stack []string) {
		// IterateStacks yields stacks in leaf-to-root order;
		// model.Tree.InsertStack expects root-to-leaf.
		slices.Reverse(stack)
		t.InsertStack(int64(self), stack...)
	})
	return t, nil
}

func parse(p *AdHocProfile, profileType *string, limits convert.Limits) (fg *flamebearer.FlamebearerProfile, profileTypes []string, err error) {
	base64decoded, err := base64.StdEncoding.DecodeString(p.Data)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to upload profile")
	}

	f := convert.ProfileFile{
		Name:     p.Name,
		TypeData: convert.ProfileFileTypeData{},
		Data:     base64decoded,
	}

	profiles, err := convert.FlamebearerFromFile(f, limits)
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
