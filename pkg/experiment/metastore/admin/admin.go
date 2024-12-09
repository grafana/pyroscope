package admin

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/discovery"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

type formActionHandler func(http.ResponseWriter, *http.Request, string) error

type MetastoreAdmin struct {
	leaderClient raftnodepb.RaftNodeOpsServiceClient
	logger       log.Logger
	clients      []raftnodepb.RaftNodeOpsServiceClient
	clientsMu    sync.Mutex

	disc discovery.Discovery

	servers        []discovery.Server
	actionHandlers map[string]formActionHandler
}

func NewAdmin(
	client raftnodepb.RaftNodeOpsServiceClient,
	logger log.Logger,
	metastoreAddress string,
) (*MetastoreAdmin, error) {
	adm := &MetastoreAdmin{
		leaderClient:   client,
		logger:         logger,
		actionHandlers: make(map[string]formActionHandler),
	}
	adm.addFormActionHandlers()

	disc, err := discovery.NewDiscovery(logger, metastoreAddress, nil)
	if err != nil {
		return nil, err
	}
	disc.Subscribe(adm)

	return adm, nil
}

func (a *MetastoreAdmin) Servers(servers []discovery.Server) {
	a.servers = servers
	slices.SortFunc(a.servers, func(a, b discovery.Server) int {
		return strings.Compare(string(a.Raft.ID), string(b.Raft.ID))
	})
	a.clientsMu.Lock()
	defer a.clientsMu.Unlock()

	a.clients = slices.Grow(a.clients, len(a.servers))[:0]
	for _, s := range a.servers {
		c, err := a.newClient(s.ResolvedAddress)
		if err != nil {
			level.Error(a.logger).Log("msg", "failed to create client", "server", s, "err", err)
		}
		a.clients = append(a.clients, c)
	}
}

func (a *MetastoreAdmin) NodeListHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			for fieldName, handler := range a.actionHandlers {
				field := r.FormValue(fieldName)
				if field != "" {
					if err := a.canPerformOperation(r.Context(), fieldName, field); err != nil {
						httputil.Error(w, err)
						return
					}
					if err := handler(w, r, field); err != nil {
						httputil.Error(w, err)
						return
					}
					w.Header().Set("Location", "#")
					w.WriteHeader(http.StatusFound)
					return
				}
			}
		}

		raftState, err := a.fetchRaftState(r.Context())
		if err != nil {
			httputil.Error(w, err)
			return
		}

		err = pageTemplates.nodesTemplate.Execute(w, nodesPageContent{
			DiscoveredServers: a.servers,
			Raft:              raftState,
			Now:               time.Now().UTC(),
		})
		if err != nil {
			httputil.Error(w, err)
		}
	})
}

func (a *MetastoreAdmin) fetchRaftState(ctx context.Context) (*raftNodeState, error) {
	leaderId := ""
	maxLeaderCount := 0
	leaderCounts := make(map[string]int)

	numRaftNodes := 0
	nodes := make([]*metastoreNode, 0, len(a.servers))

	for i, s := range a.servers {
		cl := a.clients[i]
		if cl == nil {
			level.Warn(a.logger).Log("msg", "missing client for server", "server", s)
			continue
		}
		node := &metastoreNode{
			DiscoveryServerId: string(s.Raft.ID),
			ResolvedAddress:   s.ResolvedAddress,
		}
		nodes = append(nodes, node)

		res, err := cl.NodeInfo(ctx, &raftnodepb.NodeInfoRequest{})
		if err != nil {
			level.Warn(a.logger).Log("msg", "error fetching node info", "server", s, "err", err)
			continue
		}
		nInfo := res.Node

		node.RaftServerId = nInfo.ServerId
		node.Member = nInfo.LeaderId != ""
		node.State = nInfo.State
		node.ObservedLeader = nInfo.LeaderId
		node.CurrentTerm = nInfo.CurrentTerm
		node.LastIndex = nInfo.LastIndex
		node.CommitIndex = nInfo.CommitIndex
		node.AppliedIndex = nInfo.AppliedIndex
		node.BuildVersion = nInfo.BuildVersion
		node.BuildRevision = nInfo.BuildRevision
		node.Stats = make(map[string]string)
		for i, n := range nInfo.Stats.Name {
			node.Stats[n] = nInfo.Stats.Value[i]
		}

		if node.Member {
			numRaftNodes++
		}

		// Each node has its own view of the world and can be out of sync with the rest.
		// We choose the leader to be the one that is observed as the leader the most times.
		leaderCounts[node.ObservedLeader]++
		if leaderCounts[node.ObservedLeader] > maxLeaderCount && node.ObservedLeader != "" {
			maxLeaderCount = leaderCounts[node.ObservedLeader]
			leaderId = node.ObservedLeader
		}
	}
	return &raftNodeState{
		Nodes:    nodes,
		LeaderId: leaderId,
		NumNodes: numRaftNodes,
	}, nil
}

func (a *MetastoreAdmin) newClient(address string) (raftnodepb.RaftNodeOpsServiceClient, error) {
	// TODO aleks-p: do we need more configuration here?
	client, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return raftnodepb.NewRaftNodeOpsServiceClient(client), nil
}

func (a *MetastoreAdmin) addFormActionHandlers() {
	a.actionHandlers["add"] = func(w http.ResponseWriter, r *http.Request, serverId string) error {
		_, err := a.leaderClient.AddNode(r.Context(), &raftnodepb.AddNodeRequest{ServerId: serverId})
		return err
	}
	a.actionHandlers["remove"] = func(w http.ResponseWriter, r *http.Request, serverId string) error {
		raftState, err := a.fetchRaftState(r.Context())
		if err != nil {
			return err
		}
		if raftState.LeaderId == serverId {
			// In theory, this shouldn't be needed since Raft should elect a new leader upon removal.
			// In practice, a successful leader election doesn't always occur and demoting first is safer.
			_, err = a.leaderClient.DemoteLeader(r.Context(), &raftnodepb.DemoteLeaderRequest{ServerId: serverId})
			if err != nil {
				return err
			}
		}
		_, err = a.leaderClient.RemoveNode(r.Context(), &raftnodepb.RemoveNodeRequest{ServerId: serverId})
		return err
	}
	a.actionHandlers["promote"] = func(w http.ResponseWriter, r *http.Request, serverId string) error {
		_, err := a.leaderClient.PromoteToLeader(r.Context(), &raftnodepb.PromoteToLeaderRequest{ServerId: serverId})
		return err
	}
	a.actionHandlers["demote"] = func(w http.ResponseWriter, r *http.Request, serverId string) error {
		_, err := a.leaderClient.DemoteLeader(r.Context(), &raftnodepb.DemoteLeaderRequest{ServerId: serverId})
		return err
	}
}

// The admin page can be open for a long time before an action is executed and the Raft state when the page is viewed
// can be different from the current state. Therefore, any time an operation is invoked we fetch the entire state and
// verify that the operation still makes sense.
//
// Alternatively, we could send along the state and verify that it hasn't changed.
func (a *MetastoreAdmin) canPerformOperation(ctx context.Context, operation string, serverId string) error {
	raftState, err := a.fetchRaftState(ctx)
	if err != nil {
		return err
	}
	var targetNode *metastoreNode
	for _, node := range raftState.Nodes {
		if node.RaftServerId == serverId {
			targetNode = node
			break
		}
	}
	if targetNode == nil {
		return fmt.Errorf("node %s could not be found", serverId)
	}
	switch operation {
	case "add":
		if targetNode.ObservedLeader != "" {
			return fmt.Errorf("node is already a member")
		}
	case "remove":
		if targetNode.ObservedLeader == "" {
			return fmt.Errorf("node is not a member")
		}
	case "demote":
		if targetNode.RaftServerId != raftState.LeaderId {
			return fmt.Errorf("node is not the leader")
		}
	case "promote":
		if targetNode.ObservedLeader == "" {
			return fmt.Errorf("node is not a member")
		}
		if targetNode.RaftServerId == raftState.LeaderId {
			return fmt.Errorf("node is already the leader")
		}
	default:
		return fmt.Errorf("unknown operation")
	}
	return nil
}
