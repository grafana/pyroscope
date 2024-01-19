package version

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/kv/codec"
	"github.com/grafana/dskit/kv/memberlist"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	versionv1 "github.com/grafana/pyroscope/api/gen/proto/go/version/v1"
	"github.com/grafana/pyroscope/pkg/util"
)

type dnsProviderMock struct {
	resolved []string
}

func (p *dnsProviderMock) Resolve(ctx context.Context, addrs []string) error {
	p.resolved = addrs
	return nil
}

func (p dnsProviderMock) Addresses() []string {
	return p.resolved
}

func createMemberlist(t *testing.T, port, memberID int) *memberlist.KV {
	t.Helper()
	var cfg memberlist.KVConfig
	flagext.DefaultValues(&cfg)
	cfg.TCPTransport = memberlist.TCPTransportConfig{
		BindAddrs: []string{"127.0.0.1"},
		BindPort:  0,
	}
	cfg.GossipInterval = 10 * time.Millisecond
	cfg.GossipNodes = 4
	cfg.PushPullInterval = 10 * time.Millisecond
	cfg.NodeName = fmt.Sprintf("Member-%d", memberID)
	cfg.Codecs = []codec.Codec{GetCodec()}

	mkv := memberlist.NewKV(cfg, log.NewNopLogger(), &dnsProviderMock{}, nil)
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), mkv))
	if port != 0 {
		_, err := mkv.JoinMembers([]string{fmt.Sprintf("127.0.0.1:%d", port)})
		require.NoError(t, err, "%s failed to join the cluster: %v", memberID, err)
	}
	t.Cleanup(func() {
		_ = services.StopAndAwaitTerminated(context.TODO(), mkv)
	})
	return mkv
}

func setupTests(t *testing.T) int {
	t.Helper()
	heartbeatInterval = 100 * time.Millisecond
	instanceTimeout = 500 * time.Millisecond
	initMKV := createMemberlist(t, 0, 0)
	return initMKV.GetListeningPort()
}

func TestVersionsSingle(t *testing.T) {
	var (
		port = setupTests(t)
		ctx  = context.Background()
		req  = &connect.Request[versionv1.VersionRequest]{}
	)

	svc, err := New(util.CommonRingConfig{
		InstanceID:   "1",
		InstanceAddr: "0.0.0.0",
		InstancePort: 1,
		KVStore: kv.Config{
			Store: "memberlist",
			StoreConfig: kv.StoreConfig{
				MemberlistKV: func() (*memberlist.KV, error) {
					return createMemberlist(t, port, 1), nil
				},
			},
		},
	}, log.NewNopLogger(), prometheus.NewRegistry())

	require.NoError(t, err)

	resp, err := svc.Version(ctx, req)
	require.NoError(t, err)
	require.Equal(t, uint64(0), resp.Msg.QuerierAPI)

	// start the service
	require.NoError(t, services.StartAndAwaitRunning(ctx, svc))

	require.EventuallyWithT(t, func(t *assert.CollectT) {
		resp, err := svc.Version(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, uint64(1), resp.Msg.QuerierAPI)
	}, 1*time.Second, 500*time.Millisecond)

	require.NoError(t, services.StopAndAwaitTerminated(ctx, svc))
	svc.Shutdown()
}

func TestVersionsMultiple(t *testing.T) {
	var (
		port = setupTests(t)
		ctx  = context.Background()
		req  = &connect.Request[versionv1.VersionRequest]{}
	)

	svcs := make([]*Service, 0, 3)
	for i := 0; i < 3; i++ {
		svc, err := New(util.CommonRingConfig{
			InstanceID: fmt.Sprintf("%d", i),
			KVStore: kv.Config{
				Store: "memberlist",
				StoreConfig: kv.StoreConfig{
					MemberlistKV: func() (*memberlist.KV, error) {
						return createMemberlist(t, port, i), nil
					},
				},
			},
		}, log.NewNopLogger(), prometheus.NewRegistry())
		require.NoError(t, err)
		svcs = append(svcs, svc)
	}
	svcs[0].version = 1
	svcs[1].version = 2
	svcs[2].version = 2

	expectVersion := func(t *testing.T, expected uint64) {
		t.Helper()
		require.EventuallyWithT(t, func(t *assert.CollectT) {
			resp, err := svcs[0].Version(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, expected, resp.Msg.QuerierAPI)
		}, 3*time.Second, 100*time.Millisecond)
	}

	expectVersion(t, 0)

	require.NoError(t, services.StartAndAwaitRunning(ctx, svcs[0]))
	expectVersion(t, 1)
	require.NoError(t, services.StartAndAwaitRunning(ctx, svcs[1]))
	expectVersion(t, 1)
	require.NoError(t, services.StartAndAwaitRunning(ctx, svcs[2]))
	expectVersion(t, 1)
	// wait for the version to be propagated
	svcs[0].Shutdown()
	expectVersion(t, 2)
}

var nowTs = time.Now().UnixNano()

func TestMerge(t *testing.T) {
	now = func() time.Time {
		return time.Unix(0, nowTs)
	}
	t.Cleanup(func() {
		now = time.Now
	})

	for name, tc := range map[string]struct {
		base     *Versions
		incoming memberlist.Mergeable

		expected memberlist.Mergeable
	}{
		"empty": {
			base:     nil,
			incoming: nil,
			expected: nil,
		},
		"empty base": {
			base:     nil,
			incoming: &Versions{},
			expected: &Versions{},
		},
		"empty incoming": {
			base:     &Versions{},
			incoming: nil,
			expected: nil,
		},
		"equal": {
			base: createVersions(t,
				createVersion(t, "1", 1),
				createVersion(t, "2", 2),
			),
			incoming: createVersions(t,
				createVersion(t, "1", 1),
				createVersion(t, "2", 2),
			),
			expected: nil,
		},
		"newer": {
			base: createVersions(t,
				createVersion(t, "1", 1),
				createVersion(t, "2", 2),
			),
			incoming: createVersions(t,
				createVersion(t, "1", 1),
				createVersion(t, "2", 3),
			),
			expected: createVersions(t,
				createVersion(t, "2", 3),
			),
		},
		"instance added": {
			base: createVersions(t,
				createVersion(t, "1", 1),
			),
			incoming: createVersions(t,
				createVersion(t, "1", 1),
				createVersion(t, "2", 2),
			),
			expected: createVersions(t,
				createVersion(t, "2", 2),
			),
		},
		"instance removed": {
			base: createVersions(t,
				createVersion(t, "1", 1),
				createVersion(t, "2", 2),
			),
			incoming: createVersions(t,
				createVersion(t, "1", 1),
			),
			expected: createVersions(t,
				createLeftVersion(t, "2", nowTs),
			),
		},
		"instance removed and added": {
			base: createVersions(t,
				createVersion(t, "1", 1),
				createVersion(t, "2", 2),
			),
			incoming: createVersions(t,
				createVersion(t, "3", 3),
				createVersion(t, "2", 2),
			),
			expected: createVersions(t,
				createVersion(t, "3", 3),
				createLeftVersion(t, "1", nowTs),
			),
		},
	} {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			change, err := tc.base.Merge(tc.incoming, true)
			require.NoError(t, err)
			require.Equal(t, tc.expected, change)
		})
	}
}

func createLeftVersion(t *testing.T, id string, ts int64) *versionv1.InstanceVersion {
	t.Helper()
	return &versionv1.InstanceVersion{
		ID:        id,
		Timestamp: ts,
		Left:      true,
	}
}

func createVersion(t *testing.T, id string, ts int64) *versionv1.InstanceVersion {
	t.Helper()
	return &versionv1.InstanceVersion{
		ID:        id,
		Timestamp: ts,
	}
}

func createVersions(t *testing.T, instances ...*versionv1.InstanceVersion) *Versions {
	t.Helper()
	res := &Versions{
		Versions: &versionv1.Versions{
			Instances: map[string]*versionv1.InstanceVersion{},
		},
	}
	for _, inst := range instances {
		res.Instances[inst.ID] = inst
	}
	return res
}
