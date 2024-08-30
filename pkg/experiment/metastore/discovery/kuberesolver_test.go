package discovery

import (
	kuberesolver2 "github.com/grafana/pyroscope/pkg/experiment/metastore/discovery/kuberesolver"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestName(t *testing.T) {
	l := testutil.NewLogger(t)

	//srv := NewServ
	client := kuberesolver2.NewInsecureK8sClient("http://localhost:8080")
	target := "kubernetes://kubernetes:///pyroscope-metastore-headless.pyroscope-test:9095"

	discovery, err := NewKubeResolverDiscovery(l, target, client, UpdateFunc(func(servers []Server) {

	}))
	require.NoError(t, err)

	defer discovery.Close()

}
