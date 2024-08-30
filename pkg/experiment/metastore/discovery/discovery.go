package discovery

import "github.com/hashicorp/raft"

type Updates interface {
	Servers(servers []Server)
}

type UpdateFunc func(servers []Server)

func (f UpdateFunc) Servers(servers []Server) {
	f(servers)
}

type Server struct {
	Raft raft.Server
	IP   string
}

type Discovery interface {
	//GetServers() []Server
	Close()
}
