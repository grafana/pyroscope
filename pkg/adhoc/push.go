package adhoc

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/server"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/process"
)

type push struct {
	args    []string
	handler http.Handler
	logger  *logrus.Logger
}

func newPush(_ *config.Adhoc, args []string, st *storage.Storage, logger *logrus.Logger) (runner, error) {
	e, err := exporter.NewExporter(config.MetricsExportRules{}, prometheus.DefaultRegisterer)
	if err != nil {
		return nil, err
	}
	p := parser.New(logger, st, e)
	return push{
		args:    args,
		handler: server.NewIngestHandler(logger, p, func(_ *parser.PutInput) {}),
		logger:  logger,
	}, nil
}

func (p push) Run() error {
	http.Handle("/ingest", p.handler)
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
	// Note that we don't specify which signals to be sent: any signal to be
	// relayed to the child process (including SIGINT and SIGTERM).
	signal.Notify(c)
	env := fmt.Sprintf("PYROSCOPE_ADHOC_SERVER_ADDRESS=http://localhost:%d", listener.Addr().(*net.TCPAddr).Port)
	cmd := exec.Command(p.args[0], p.args[1:]...)
	cmd.Env = append(os.Environ(), env)
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
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case s := <-c:
			_ = process.SendSignal(cmd.Process, s)
		case err := <-done:
			return err
		case <-ticker.C:
			if !process.Exists(cmd.Process.Pid) {
				logrus.Debug("child process exited")
				return cmd.Wait()
			}
		}
	}
}
