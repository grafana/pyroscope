package admin

import (
	"net"
	"net/http"
	"os"

	"github.com/sirupsen/logrus"
)

// TODO get this from parameters?
const socketAddr = "/tmp/pyroscope.sock"

type Controller struct {
	log   *logrus.Logger
	close chan struct{}
}

type Config struct {
	*logrus.Logger
}

func NewController(c Config) *Controller {
	ctrl := &Controller{
		log:   c.Logger,
		close: make(chan struct{}),
	}

	return ctrl
}

func (c *Controller) Start() error {
	if err := os.RemoveAll(socketAddr); err != nil {
		return err
	}

	admin := &Admin{}
	mux := http.NewServeMux()

	// Routes
	mux.HandleFunc("/foo", admin.GetApps)

	adminServer := http.Server{Handler: mux}
	adminListener, err := net.Listen("unix", socketAddr)
	if err != nil {
		return err
	}

	// TODO
	// is this blocking?
	adminServer.Serve(adminListener)
	return nil
}

type Admin struct{}

func (a *Admin) GetApps(w http.ResponseWriter, r *http.Request) {
	println("get apps")
}
