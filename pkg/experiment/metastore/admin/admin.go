package admin

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/discovery"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

type formActionHandler func(http.ResponseWriter, *http.Request, string) error

type Admin struct {
	service services.Service

	logger log.Logger

	servers      []discovery.Server
	leaderClient raftnodepb.RaftNodeServiceClient

	actionHandlers map[string]formActionHandler
}

func (a *Admin) Service() services.Service {
	return a.service
}

type metastoreClient struct {
	raftnodepb.RaftNodeServiceClient
	conn *grpc.ClientConn
}

func New(
	client raftnodepb.RaftNodeServiceClient,
	logger log.Logger,
	metastoreAddress string,
) (*Admin, error) {
	adm := &Admin{
		leaderClient:   client,
		logger:         logger,
		actionHandlers: make(map[string]formActionHandler),
	}
	adm.addFormActionHandlers()
	adm.service = services.NewIdleService(adm.starting, adm.stopping)

	disc, err := discovery.NewDiscovery(logger, metastoreAddress, nil)
	if err != nil {
		return nil, err
	}
	disc.Subscribe(adm)

	return adm, nil
}

func (a *Admin) starting(context.Context) error { return nil }
func (a *Admin) stopping(error) error           { return nil }

func (a *Admin) Servers(servers []discovery.Server) {
	a.servers = servers
	slices.SortFunc(a.servers, func(a, b discovery.Server) int {
		return strings.Compare(string(a.Raft.ID), string(b.Raft.ID))
	})
}

func (a *Admin) NodeListHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			for fieldName, handler := range a.actionHandlers {
				field := r.FormValue(fieldName)
				if field != "" {
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

func (a *Admin) fetchRaftState(ctx context.Context) (*raftNodeState, error) {
	leaderId := ""
	maxLeaderCount := 0
	leaderCounts := make(map[string]int)

	numRaftNodes := 0
	nodes := make([]*metastoreNode, 0, len(a.servers))

	for _, s := range a.servers {
		cl, err := newClient(s.ResolvedAddress)
		if err != nil {
			level.Warn(a.logger).Log("msg", "missing client for server", "server", s)
			continue
		}
		node := &metastoreNode{
			DiscoveryServerId: string(s.Raft.ID),
			ResolvedAddress:   s.ResolvedAddress,
		}
		nodes = append(nodes, node)

		res, err := cl.NodeInfo(ctx, &raftnodepb.NodeInfoRequest{})
		_ = cl.conn.Close()

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

func newClient(address string) (*metastoreClient, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &metastoreClient{
		RaftNodeServiceClient: raftnodepb.NewRaftNodeServiceClient(conn),
		conn:                  conn,
	}, nil
}

func (a *Admin) addFormActionHandlers() {
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
			// Without this, the removed node cannot be re-added later because Raft gets shuts down on the node.
			// Alternatively, we could set raftConfig.ShutdownOnRemove to false.
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
