package cluster

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/grafana/pyroscope/pkg/metastore/raftnode/raftnodepb"
)

func WithV2() ClusterOption {
	return func(c *Cluster) {
		c.v2 = true
		c.expectedComponents = []string{
			"distributor",
			"distributor",
			"segment-writer",
			"segment-writer",
			"metastore",
			"metastore",
			"metastore",
			"query-frontend",
			"query-backend",
			"compaction-worker",
		}
	}
}

func (c *Cluster) metastoreConfig() (string, error) {
	cfgPath := filepath.Join(c.tmpDir, "metastore.yaml")

	// check if the file exists
	if _, err := os.Stat(cfgPath); err == nil {
		return cfgPath, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	// ensure compaction worker are picking up l0 compaction straight away
	metastoreConfig := `
metastore:
    levels:
        - maxblocks: 20
          maxage: 2000000000 # 2 seconds
`
	tmpFile, err := os.Create(cfgPath)
	if err != nil {
		return "", err
	}
	if _, err := tmpFile.Write([]byte(metastoreConfig)); err != nil {
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}
	return tmpFile.Name(), nil
}

func (c *Cluster) metastores() []*Component {
	metastores := make([]*Component, 0, len(c.perTarget["metastore"]))
	for _, compidx := range c.perTarget["metastore"] {
		metastores = append(metastores, c.Components[compidx])
	}
	return metastores
}

func (c *Cluster) metastoreExpectedLeader() *Component {
	metastores := c.metastores()
	return metastores[len(metastores)-1]
}

func (c *Cluster) CompactionJobsFinished(ctx context.Context) (float64, error) {
	leader := c.metastoreExpectedLeader()

	floatCh := make(chan float64, 1)
	check := leader.checkMetrics().
		addRetrieveValue(floatCh, "pyroscope_metastore_compaction_scheduler_queue_completed_jobs_total", "level", "0")

	if err := check.run(ctx); err != nil {
		return 0, err
	}
	close(floatCh)

	sum := 0.0
	found := false
	for v := range floatCh {
		found = true
		sum += v
	}
	if !found {
		return 0, fmt.Errorf("no value received")
	}
	return sum, nil
}

func (c *Cluster) v2Prepare(_ context.Context, memberlistJoin []string) error {
	metastoreLeader := c.metastoreExpectedLeader()

	for _, comp := range c.Components {
		if err := c.v2PrepareComponent(comp, metastoreLeader); err != nil {
			return err
		}

		// handle memberlist join
		for _, m := range memberlistJoin {
			comp.flags = append(comp.flags, fmt.Sprintf("-memberlist.join=%s", m))
		}
	}

	return nil
}

func (c *Cluster) v2PrepareComponent(comp *Component, metastoreLeader *Component) error {
	dataDir := c.dataDir(comp)

	comp.cfg.V2 = true
	comp.flags = c.commonFlags(comp)

	comp.flags = append(comp.flags,
		"-enable-query-backend=true",
		"-write-path=segment-writer",
		"-metastore.min-ready-duration=0",
		fmt.Sprintf("-metastore.address=%s:%d/%s", listenAddr, metastoreLeader.grpcPort, metastoreLeader.nodeName()),
	)

	if c.debuginfodURL != "" && comp.Target == "query-frontend" {
		comp.flags = append(comp.flags,
			fmt.Sprintf("-symbolizer.debuginfod-url=%s", c.debuginfodURL),
			"-symbolizer.enabled=true",
		)
	}

	if comp.Target == "segment-writer" {
		comp.flags = append(comp.flags,
			"-segment-writer.num-tokens=1",
			"-segment-writer.min-ready-duration=0",
			"-segment-writer.lifecycler.addr="+listenAddr,
			"-segment-writer.lifecycler.ID="+comp.nodeName(),
			"-segment-writer.heartbeat-period=1s",
		)
	}

	if comp.Target == "compaction-worker" {
		comp.flags = append(comp.flags,
			"-compaction-worker.job-concurrency=20",
			"-compaction-worker.job-poll-interval=1s",
		)
	}

	// register query-backends in the frontend and themselves
	if comp.Target == "query-frontend" || comp.Target == "query-backend" {
		for _, compidx := range c.perTarget["query-backend"] {
			comp.flags = append(comp.flags,
				fmt.Sprintf("-query-backend.address=%s:%d", listenAddr, c.Components[compidx].grpcPort),
			)
		}
	}

	// handle metastore folders and ports
	if comp.Target == "metastore" {
		cfgPath, err := c.metastoreConfig()
		if err != nil {
			return err
		}
		comp.flags = append(comp.flags,
			fmt.Sprint("-config.file=", cfgPath),
			fmt.Sprintf("-metastore.data-dir=%s", dataDir+"../metastore-ephemeral"),
			fmt.Sprintf("-metastore.raft.dir=%s", dataDir+"../metastore-raft"),
			fmt.Sprintf("-metastore.raft.snapshots-dir=%s", dataDir+"../metastore-snapshots"),
			fmt.Sprintf("-metastore.raft.bind-address=%s:%d", listenAddr, comp.raftPort),
			fmt.Sprintf("-metastore.raft.advertise-address=%s:%d", listenAddr, comp.raftPort),
			fmt.Sprintf("-metastore.raft.server-id=%s", comp.nodeName()),
			fmt.Sprintf("-metastore.raft.bootstrap-expect-peers=%d", len(c.perTarget[comp.Target])),
		)

		// add bootstrap peers
		for _, compidx := range c.perTarget[comp.Target] {
			peer := c.Components[compidx]
			comp.flags = append(comp.flags,
				fmt.Sprintf("-metastore.raft.bootstrap-peers=%s:%d/%s", listenAddr, peer.raftPort, peer.nodeName()),
			)
		}
	}

	return nil
}

func (c *Cluster) v2ReadyCheckComponent(ctx context.Context, t *Component) (bool, error) {
	switch t.Target {
	case "metastore":
		return true, t.metastoreReadyCheck(ctx, c.metastores(), c.metastoreExpectedLeader())
	case "distributor":
		return true, t.distributorReadyCheck(ctx, 0, len(c.perTarget["segment-writer"]), len(c.perTarget["distributor"]))
	}
	return false, nil
}

// for the metastore, we need to check that the first replica is the leader, as this is configured statically as the client for other components.
func (comp *Component) metastoreReadyCheck(ctx context.Context, metastores []*Component, expectedLeader *Component) error {
	expectedPeers := len(metastores)

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	cc, err := grpc.NewClient(fmt.Sprintf("%s:%d", listenAddr, comp.grpcPort), opts...)
	if err != nil {
		return err
	}

	client := raftnodepb.NewRaftNodeServiceClient(cc)

	nodeInfo, err := client.NodeInfo(ctx, &raftnodepb.NodeInfoRequest{})
	if err != nil {
		return err
	}

	// only ready once all peers are here
	if len(nodeInfo.Node.Peers) != expectedPeers {
		return fmt.Errorf("unexpected peer count: exp=%d actual=%d", expectedPeers, len(nodeInfo.Node.Peers))
	}

	// only ready once leader is known
	if nodeInfo.Node.LeaderId == "" {
		return fmt.Errorf("leader not known on node %s", comp.nodeName())
	}

	// exit if we are not the leader
	if nodeInfo.Node.LeaderId != nodeInfo.Node.ServerId {
		return nil
	}

	// if we are replica 0 we are done as we are already leader
	if comp.replica == expectedPeers-1 {
		return nil
	}

	// promote last metastore to new leader
	_, err = client.PromoteToLeader(ctx, &raftnodepb.PromoteToLeaderRequest{
		ServerId:    fmt.Sprintf("%s:%d/%s", listenAddr, expectedLeader.raftPort, expectedLeader.nodeName()),
		CurrentTerm: nodeInfo.Node.CurrentTerm,
	})
	return err
}

func (c *Cluster) GetMetastoreRaftNodeClient() (raftnodepb.RaftNodeServiceClient, error) {
	leader := c.metastoreExpectedLeader()
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	cc, err := grpc.NewClient(fmt.Sprintf("127.0.0.1:%d", leader.grpcPort), opts...)
	if err != nil {
		return nil, err
	}

	return raftnodepb.NewRaftNodeServiceClient(cc), nil
}

func (c *Cluster) AddMetastoreWithAutoJoin(ctx context.Context) error {
	leader := c.metastoreExpectedLeader()

	comp := newComponent("metastore")
	comp.replica = len(c.perTarget["metastore"])
	c.Components = append(c.Components, comp)
	c.perTarget["metastore"] = append(c.perTarget["metastore"], len(c.Components)-1)

	if err := c.v2PrepareComponent(comp, leader); err != nil {
		return err
	}
	comp.flags = append(comp.flags, "-metastore.raft.auto-join=true")

	p, err := comp.start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start component: %w", err)
	}
	comp.p = p

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := p.Run(); err != nil {
			fmt.Printf("metastore with auto-join stopped with error: %v\n", err)
		}
	}()

	return nil
}
