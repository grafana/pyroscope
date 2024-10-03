package discovery

import (
	"fmt"
	"github.com/hashicorp/raft"
)

type Updates interface {
	Servers(servers []Server)
}

type UpdateFunc func(servers []Server)

func (f UpdateFunc) Servers(servers []Server) {
	f(servers)
}

type Server struct {
	Raft            raft.Server
	ResolvedAddress string
}

func (s *Server) String() string {
	return fmt.Sprintf("Server{id: %s, Address %s ResolvedAddress: %v}", s.Raft.ID, s.Raft.Address, s.ResolvedAddress)
}

type Discovery interface {
	Subscribe(updates Updates)
	Rediscover()
	Close()
}
