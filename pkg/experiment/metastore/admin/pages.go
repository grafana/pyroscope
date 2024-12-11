package admin

import (
	_ "embed"
	"html/template"
	"time"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/discovery"
)

//go:embed metastore.nodes.gohtml
var nodesPageHtml string

type metastoreNode struct {
	// from Discovery
	DiscoveryServerId string
	RaftServerId      string
	ResolvedAddress   string

	// from Raft
	Member        bool
	State         string
	CommitIndex   uint64
	AppliedIndex  uint64
	LastIndex     uint64
	LeaderId      string
	ConfigIndex   uint64
	NumPeers      int
	CurrentTerm   uint64
	BuildVersion  string
	BuildRevision string
	Stats         map[string]string
}

type raftNodeState struct {
	Nodes           []*metastoreNode
	ObservedLeaders map[string]int
	CurrentTerm     uint64
	NumNodes        int
}

type nodesPageContent struct {
	DiscoveredServers []discovery.Server
	Raft              *raftNodeState
	Now               time.Time
}

type templates struct {
	nodesTemplate *template.Template
}

var pageTemplates = initTemplates()

func initTemplates() *templates {
	nodesTemplate := template.New("nodes")
	template.Must(nodesTemplate.Parse(nodesPageHtml))
	t := &templates{
		nodesTemplate: nodesTemplate,
	}
	return t
}
