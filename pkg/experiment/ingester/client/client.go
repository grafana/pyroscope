package sewgmentwriterclient

import (
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
)

type Client struct {
	ring ring.ReadRing
	pool *ring_client.Pool
}
