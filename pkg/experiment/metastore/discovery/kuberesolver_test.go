package discovery

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/stretchr/testify/require"

	kuberesolver2 "github.com/grafana/pyroscope/pkg/experiment/metastore/discovery/kuberesolver"
)

func TestDebugLocalhost(t *testing.T) {
	t.Skip()
	client := kuberesolver2.NewInsecureK8sClient("http://localhost:8080")
	target := "kubernetes:///pyroscope-metastore-headless.pyroscope-test:9095"

	discovery, err := NewKubeResolverDiscovery(testutil.NewLogger(t), target, client)
	require.NoError(t, err)
	discovery.Subscribe(UpdateFunc(func(servers []Server) {
		fmt.Printf("servers: %+v\n", servers)
	}))

	defer discovery.Close()
	time.Sleep(555 * time.Second)

}

func TestConvert(t *testing.T) {
	e := kuberesolver2.Endpoints{
		Kind:       "Endpoints",
		ApiVersion: "v1",
		Metadata: kuberesolver2.Metadata{
			Name:            "pyroscope-metastore-headless",
			Namespace:       "pyroscope-test",
			ResourceVersion: "1013720",
			Labels:          map[string]string{},
		},
		Subsets: []kuberesolver2.Subset{
			{
				Addresses: []kuberesolver2.Address{
					{
						IP: "10.244.1.5",
						TargetRef: &kuberesolver2.ObjectReference{
							Kind:      "Pod",
							Name:      "pyroscope-metastore-0",
							Namespace: "pyroscope-test",
						},
					},
					{
						IP: "10.244.2.7",
						TargetRef: &kuberesolver2.ObjectReference{
							Kind:      "Pod",
							Name:      "pyroscope-metastore-1",
							Namespace: "pyroscope-test",
						},
					},
					{
						IP: "10.244.3.7",
						TargetRef: &kuberesolver2.ObjectReference{
							Kind:      "Pod",
							Name:      "pyroscope-metastore-2",
							Namespace: "pyroscope-test",
						},
					},
				},
				Ports: []kuberesolver2.Port{
					{
						Name: "http2",
						Port: 4040,
					},
					{
						Name: "raft",
						Port: 9099,
					},
					{
						Name: "grpc",
						Port: 9095,
					},
				},
			},
		},
	}

	servers := convertEndpoints(e, targetInfo{
		namespace: "pyroscope-test",
		service:   "pyroscope-metastore-headless",
		port:      "9095",
	})
	expected := []Server{
		{
			ResolvedAddress: "10.244.1.5:9095",
			Raft: raft.Server{
				ID:      raft.ServerID("pyroscope-metastore-0.pyroscope-metastore-headless.pyroscope-test.svc.cluster.local.:9095"),
				Address: raft.ServerAddress("pyroscope-metastore-0.pyroscope-metastore-headless.pyroscope-test.svc.cluster.local.:9095"),
			},
		},
		{
			ResolvedAddress: "10.244.2.7:9095",
			Raft: raft.Server{
				ID:      raft.ServerID("pyroscope-metastore-1.pyroscope-metastore-headless.pyroscope-test.svc.cluster.local.:9095"),
				Address: raft.ServerAddress("pyroscope-metastore-1.pyroscope-metastore-headless.pyroscope-test.svc.cluster.local.:9095"),
			},
		},
		{
			ResolvedAddress: "10.244.3.7:9095",
			Raft: raft.Server{
				ID:      raft.ServerID("pyroscope-metastore-2.pyroscope-metastore-headless.pyroscope-test.svc.cluster.local.:9095"),
				Address: raft.ServerAddress("pyroscope-metastore-2.pyroscope-metastore-headless.pyroscope-test.svc.cluster.local.:9095"),
			},
		},
	}
	require.Equal(t, expected, servers)

}
