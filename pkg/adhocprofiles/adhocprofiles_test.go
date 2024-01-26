package adhocprofiles

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
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

func TestAdHocProfiles_Upload(t *testing.T) {
	bucket := phlareobjstore.NewBucket(thanosobjstore.NewInMemBucket())
	rawProfile, err := os.ReadFile("testdata/cpu.pprof")
	require.NoError(t, err)
	encodedProfile := base64.StdEncoding.EncodeToString(rawProfile)
	type args struct {
		ctx context.Context
		c   *connect.Request[v1.AdHocProfilesUploadRequest]
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
			name: "should reject an invalid profile",
			args: args{
				ctx: tenant.InjectTenantID(context.Background(), "tenant"),
				c: connect.NewRequest(&v1.AdHocProfilesUploadRequest{
					Name:    "test",
					Profile: "123",
				}),
			},
			wantErr: true,
		},
		{
			name: "should store a valid profile",
			args: args{
				ctx: tenant.InjectTenantID(context.Background(), "tenant"),
				c: connect.NewRequest(&v1.AdHocProfilesUploadRequest{
					Name:    "test",
					Profile: encodedProfile,
				}),
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
			_, err := a.Upload(tt.args.ctx, tt.args.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("Upload() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
