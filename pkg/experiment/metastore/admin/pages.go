package admin

import (
	_ "embed"
	"html/template"
	"time"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/discovery"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
)

//go:embed metastore.nodes.gohtml
var nodesPageHtml string

//go:embed metastore.snapshots.gohtml
var snapshotsPageHtml string

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

type snapshotsPageContent struct {
	Node      *metastoreNode
	Snapshots []*raftnodepb.RaftSnapshot
	Now       time.Time
}

type templates struct {
	nodesTemplate     *template.Template
	snapshotsTemplate *template.Template
}

var pageTemplates = initTemplates()

func initTemplates() *templates {
	nodesTemplate := template.New("nodes")
	template.Must(nodesTemplate.Parse(nodesPageHtml))
	snapshotsTemplate := template.New("snapshots")
	template.Must(snapshotsTemplate.Parse(snapshotsPageHtml))
	t := &templates{
		nodesTemplate:     nodesTemplate,
		snapshotsTemplate: snapshotsTemplate,
	}
	return t
}
