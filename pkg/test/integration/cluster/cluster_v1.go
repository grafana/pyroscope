package cluster

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func WithV1() ClusterOption {
	return func(c *Cluster) {
		c.v2 = false
		c.expectedComponents = []string{
			"distributor",
			"distributor",
			"querier",
			"querier",
			"ingester",
			"ingester",
			"ingester",
			"store-gateway",
			"store-gateway",
			"store-gateway",
		}
	}
}

func WithSymbolizer(debuginfodURL string) ClusterOption {
	return func(c *Cluster) {
		c.debuginfodURL = debuginfodURL
	}
}

func (c *Cluster) v1ReadyCheckComponent(ctx context.Context, t *Component) (bool, error) {
	switch t.Target {
	case "querier":
		return true, t.querierReadyCheck(ctx, len(c.perTarget["ingester"]), len(c.perTarget["store-gateway"]))
	case "distributor":
		return true, t.distributorReadyCheck(ctx, len(c.perTarget["ingester"]), len(c.perTarget["distributor"]), 0)
	}
	return false, nil
}

func (c *Cluster) v1Prepare(_ context.Context, memberlistJoin []string) error {
	for _, comp := range c.Components {
		dataDir := c.dataDir(comp)
		compactorDir := filepath.Join(dataDir, "..", "data-compactor")
		syncDir := filepath.Join(dataDir, "..", "pyroscope-sync")

		for _, dir := range []string{compactorDir, syncDir} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}

		comp.flags = c.commonFlags(comp)
		comp.flags = append(comp.flags,
			fmt.Sprintf("-blocks-storage.bucket-store.sync-dir=%s", syncDir),
			fmt.Sprintf("-compactor.data-dir=%s", compactorDir),
			fmt.Sprintf("-pyroscopedb.data-path=%s", dataDir),
			"-distributor.replication-factor=3",
			"-store-gateway.sharding-ring.replication-factor=3",
			"-query-scheduler.ring.instance-id="+comp.nodeName(),
			"-query-scheduler.ring.instance-addr="+listenAddr,
			"-store-gateway.sharding-ring.instance-id="+comp.nodeName(),
			"-store-gateway.sharding-ring.instance-addr="+listenAddr,
			"-compactor.ring.instance-addr="+listenAddr,
			"-compactor.ring.instance-id="+comp.nodeName(),
			"-ingester.lifecycler.addr="+listenAddr,
			"-ingester.lifecycler.ID="+comp.nodeName(),
			"-ingester.min-ready-duration=0",
		)

		// handle memberlist join
		for _, m := range memberlistJoin {
			comp.flags = append(comp.flags, fmt.Sprintf("-memberlist.join=%s", m))
		}
	}
	return nil
}
