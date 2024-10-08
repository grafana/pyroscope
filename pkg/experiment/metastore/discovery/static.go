package discovery

type StaticDiscovery struct {
	servers []Server
}

func NewStaticDiscovery(servers []Server) *StaticDiscovery {
	return &StaticDiscovery{servers: servers}
}

func (s *StaticDiscovery) Subscribe(updates Updates) {
	updates.Servers(s.servers)
}

func (s *StaticDiscovery) Rediscover() {
}

func (s *StaticDiscovery) Close() {

}
