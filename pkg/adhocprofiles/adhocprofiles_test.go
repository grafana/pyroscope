package adhocprofiles

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	thanosobjstore "github.com/thanos-io/objstore"

	v1 "github.com/grafana/pyroscope/api/gen/proto/go/adhocprofiles/v1"
	phlareobjstore "github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/validation"
)

func TestAdHocProfiles_Get(t *testing.T) {
	bucket := phlareobjstore.NewBucket(thanosobjstore.NewInMemBucket())
	rawProfile, err := os.ReadFile("testdata/cpu.pprof")
	require.NoError(t, err)
	encodedProfile := base64.StdEncoding.EncodeToString(rawProfile)
	ahp := &AdHocProfile{
		Name: "cpu.pprof",
		Data: encodedProfile,
	}
	jsonProfile, _ := json.Marshal(ahp)
	_ = bucket.Upload(context.Background(), "tenant/adhoc/existing-invalid-json", bytes.NewReader([]byte{1, 2, 3}))
	_ = bucket.Upload(context.Background(), "tenant/adhoc/existing-valid-profile", bytes.NewReader(jsonProfile))
	type args struct {
		ctx context.Context
		c   *connect.Request[v1.AdHocProfilesGetRequest]
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "reject requests with missing tenant id",
			args: args{
				ctx: context.Background(),
				c:   nil,
			},
			wantErr: true,
		},
		{
			name: "return error when getting a non existing profile",
			args: args{
				ctx: tenant.InjectTenantID(context.Background(), "tenant"),
				c:   connect.NewRequest(&v1.AdHocProfilesGetRequest{Id: "non-existing-id"}),
			},
			wantErr: true,
		},
		{
			name: "return error when getting an existing invalid profile",
			args: args{
				ctx: tenant.InjectTenantID(context.Background(), "tenant"),
				c:   connect.NewRequest(&v1.AdHocProfilesGetRequest{Id: "existing-invalid-profile"}),
			},
			wantErr: true,
		},
		{
			name: "return data when getting an existing valid profile",
			args: args{
				ctx: tenant.InjectTenantID(context.Background(), "tenant"),
				c:   connect.NewRequest(&v1.AdHocProfilesGetRequest{Id: "existing-valid-profile"}),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &AdHocProfiles{
				logger: util.Logger,
				limits: validation.MockLimits{MaxFlameGraphNodesDefaultValue: 8192},
				bucket: bucket,
			}
			_, err := a.Get(tt.args.ctx, tt.args.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestAdHocProfiles_List(t *testing.T) {
	bucket := phlareobjstore.NewBucket(thanosobjstore.NewInMemBucket())
	_ = bucket.Upload(context.Background(), "tenant/adhoc/bad-id-should-be-ignored", bytes.NewReader([]byte{1}))
	_ = bucket.Upload(context.Background(), "tenant/adhoc/01HMXV8BF4EH71NBYZNPPVGJ2X-cpu.pprof", bytes.NewReader([]byte{1}))
	_ = bucket.Upload(context.Background(), "tenant/adhoc/01HMXRV02963FK36GGRE9N6MPH-heap.pprof", bytes.NewReader([]byte{1}))
	a := &AdHocProfiles{
		logger: util.Logger,
		bucket: bucket,
	}
	response, err := a.List(tenant.InjectTenantID(context.Background(), "tenant"), connect.NewRequest(&v1.AdHocProfilesListRequest{}))
	require.NoError(t, err)
	expected := []*v1.AdHocProfilesProfileMetadata{
		{
			Id:         "01HMXV8BF4EH71NBYZNPPVGJ2X-cpu.pprof",
			Name:       "cpu.pprof",
			UploadedAt: 1706103680484,
		},
		{
			Id:         "01HMXRV02963FK36GGRE9N6MPH-heap.pprof",
			Name:       "heap.pprof",
			UploadedAt: 1706101145673,
		},
	}
	require.Equal(t, connect.NewResponse(&v1.AdHocProfilesListResponse{Profiles: expected}), response)
}

func TestAdHocProfiles_Diff(t *testing.T) {
	bucket := phlareobjstore.NewBucket(thanosobjstore.NewInMemBucket())
	rawProfile, err := os.ReadFile("testdata/cpu.pprof")
	require.NoError(t, err)
	encodedProfile := base64.StdEncoding.EncodeToString(rawProfile)

	leftProfile := &AdHocProfile{Name: "left.pprof", Data: encodedProfile}
	rightProfile := &AdHocProfile{Name: "right.pprof", Data: encodedProfile}
	leftJSON, _ := json.Marshal(leftProfile)
	rightJSON, _ := json.Marshal(rightProfile)
	_ = bucket.Upload(context.Background(), "tenant/adhoc/left-profile", bytes.NewReader(leftJSON))
	_ = bucket.Upload(context.Background(), "tenant/adhoc/right-profile", bytes.NewReader(rightJSON))

	ctx := tenant.InjectTenantID(context.Background(), "tenant")
	a := &AdHocProfiles{
		logger: util.Logger,
		limits: validation.MockLimits{MaxFlameGraphNodesDefaultValue: 8192},
		bucket: bucket,
	}

	t.Run("reject requests with missing tenant id", func(t *testing.T) {
		_, err := a.Diff(context.Background(), connect.NewRequest(&v1.AdHocProfilesDiffRequest{
			LeftId:  "left-profile",
			RightId: "right-profile",
		}))
		require.Error(t, err)
	})

	t.Run("return error for invalid left id", func(t *testing.T) {
		_, err := a.Diff(ctx, connect.NewRequest(&v1.AdHocProfilesDiffRequest{
			LeftId:  "../../etc/passwd",
			RightId: "right-profile",
		}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid")
	})

	t.Run("return error for non-existing profile", func(t *testing.T) {
		_, err := a.Diff(ctx, connect.NewRequest(&v1.AdHocProfilesDiffRequest{
			LeftId:  "non-existing-id",
			RightId: "right-profile",
		}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to fetch profiles")
	})

	t.Run("return diff for valid profiles", func(t *testing.T) {
		resp, err := a.Diff(ctx, connect.NewRequest(&v1.AdHocProfilesDiffRequest{
			LeftId:  "left-profile",
			RightId: "right-profile",
		}))
		require.NoError(t, err)
		require.NotEmpty(t, resp.Msg.ProfileTypes)
		require.NotEmpty(t, resp.Msg.FlamebearerProfile)

		var fb map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(resp.Msg.FlamebearerProfile), &fb))
		metadata := fb["metadata"].(map[string]interface{})
		require.Equal(t, "double", metadata["format"])
	})

	t.Run("return diff with explicit profile type", func(t *testing.T) {
		// First, find available profile types.
		getResp, err := a.Get(ctx, connect.NewRequest(&v1.AdHocProfilesGetRequest{Id: "left-profile"}))
		require.NoError(t, err)
		require.NotEmpty(t, getResp.Msg.ProfileTypes)

		pt := getResp.Msg.ProfileTypes[0]
		resp, err := a.Diff(ctx, connect.NewRequest(&v1.AdHocProfilesDiffRequest{
			LeftId:      "left-profile",
			RightId:     "right-profile",
			ProfileType: &pt,
		}))
		require.NoError(t, err)
		require.NotEmpty(t, resp.Msg.FlamebearerProfile)

		var fb map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(resp.Msg.FlamebearerProfile), &fb))
		metadata := fb["metadata"].(map[string]interface{})
		require.Equal(t, "double", metadata["format"])
	})
}

func TestAdHocProfiles_Upload(t *testing.T) {
	overrides := validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
		defaults.MaxFlameGraphNodesDefault = 8192

		l := validation.MockDefaultLimits()
		l.MaxProfileSizeBytes = 16
		tenantLimits["tenant-16-bytes-limit"] = l

		l = validation.MockDefaultLimits()
		l.MaxProfileSizeBytes = 1600
		tenantLimits["tenant-1600-bytes-limit"] = l
	})

	bucket := phlareobjstore.NewBucket(thanosobjstore.NewInMemBucket())
	rawProfile, err := os.ReadFile("testdata/cpu.pprof")
	require.NoError(t, err)
	encodedProfile := base64.StdEncoding.EncodeToString(rawProfile)
	type args struct {
		ctx context.Context
		c   *connect.Request[v1.AdHocProfilesUploadRequest]
	}
	tests := []struct {
		name           string
		args           args
		wantErr        string
		expectedSuffix string
	}{
		{
			name: "reject requests with missing tenant id",
			args: args{
				ctx: context.Background(),
				c:   nil,
			},
			wantErr: "no org id",
		},
		{
			name: "should reject an invalid profile",
			args: args{
				ctx: tenant.InjectTenantID(context.Background(), "tenant"),
				c: connect.NewRequest(&v1.AdHocProfilesUploadRequest{
					Name:    "test",
					Profile: "123",
				}),
			},
			wantErr: "failed to parse profile",
		},
		{
			name: "should store a valid profile",
			args: args{
				ctx: tenant.InjectTenantID(context.Background(), "tenant"),
				c: connect.NewRequest(&v1.AdHocProfilesUploadRequest{
					Name:    "test.cpu.pb.gz",
					Profile: encodedProfile,
				}),
			},
			expectedSuffix: "-test.cpu.pb.gz",
		},
		{
			name: "should limit profile names to particular character set",
			args: args{
				ctx: tenant.InjectTenantID(context.Background(), "tenant"),
				c: connect.NewRequest(&v1.AdHocProfilesUploadRequest{
					Name:    "test/../../../etc/passwd",
					Profile: encodedProfile,
				}),
			},
			expectedSuffix: "-test_.._.._.._etc_passwd",
		},
		{
			name: "should enforce profile size",
			args: args{
				ctx: tenant.InjectTenantID(context.Background(), "tenant-16-bytes-limit"),
				c: connect.NewRequest(&v1.AdHocProfilesUploadRequest{
					Name:    "compressed-too-big",
					Profile: encodedProfile,
				}),
			},
			wantErr: "invalid_argument: profile payload size exceeds limit of 16 B",
		},
		{
			name: "should enforce profile size limit after decompression",
			args: args{
				// 1580 is the profile size compressed
				ctx: tenant.InjectTenantID(context.Background(), "tenant-1600-bytes-limit"),
				c: connect.NewRequest(&v1.AdHocProfilesUploadRequest{
					Name:    "decompressed-too-big",
					Profile: encodedProfile,
				}),
			},
			wantErr: "invalid_argument: uncompressed profile payload size exceeds limit of 1.6 kB",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &AdHocProfiles{
				logger: util.Logger,
				limits: overrides,
				bucket: bucket,
			}
			_, err := a.Upload(tt.args.ctx, tt.args.c)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}

			if tt.expectedSuffix != "" {
				found := false
				err := bucket.Iter(tt.args.ctx, "tenant/adhoc", func(name string) error {
					if strings.HasSuffix(name, tt.expectedSuffix) {
						found = true
					}
					return nil
				})
				require.NoError(t, err)
				require.True(t, found)
			}
		})
	}
}
