package adhoc

import (
	"fmt"
	"net"
	"net/http"
	"os"
	goexec "os/exec"
	"os/signal"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/valyala/bytebufferpool"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exec"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/server"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type push struct {
	args       []string
	storage    *storage.Storage
	logger     *logrus.Logger
	bufferPool bytebufferpool.Pool
}

func newPush(_cfg *config.Adhoc, args []string, storage *storage.Storage, logger *logrus.Logger) (runner, error) {
	return push{
		args:    args,
		storage: storage,
		logger:  logger,
	}, nil
}

func (p push) Run() error {
	exporter, err := exporter.NewExporter(config.MetricsExportRules{}, prometheus.DefaultRegisterer)
	handler := server.NewIngestHandler(p.logger, p.storage, exporter, func(_ *storage.PutInput) {})
	http.Handle("/ingest", handler)
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	p.logger.Debugf("Ingester listening to port %d", listener.Addr().(*net.TCPAddr).Port)

	done := make(chan error)
	go func() {
		done <- http.Serve(listener, nil)
	}()

	// start command
	c := make(chan os.Signal, 10)
	var cmd *goexec.Cmd
	// Note that we don't specify which signals to be sent: any signal to be
	// relayed to the child process (including SIGINT and SIGTERM).
	signal.Notify(c)
	cmd = goexec.Command(p.args[0], p.args[1:]...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PYROSCOPE_SERVER_ADDRESS=http://localhost:%d", listener.Addr().(*net.TCPAddr).Port))
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		signal.Stop(c)
		close(c)
	}()
	return exec.WaitForProcess(p.logger, cmd, c, 0, true)
}
