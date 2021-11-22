package adhoc

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
	"github.com/pyroscope-io/pyroscope/pkg/util/process"
	"github.com/sirupsen/logrus"
	"github.com/valyala/bytebufferpool"
)

type push struct {
	config     *config.AdhocRecord
	args       []string
	storage    *storage.Storage
	logger     *logrus.Logger
	bufferPool bytebufferpool.Pool
}

func newPush(config *config.AdhocRecord, args []string, storage *storage.Storage, logger *logrus.Logger) push {
	return push{
		config:  config,
		args:    args,
		storage: storage,
		logger:  logger,
	}
}

func (p push) run() error {
	http.HandleFunc("/ingest", func(w http.ResponseWriter, r *http.Request) {
		// TODO(abeaumont): The handler is copy-pasted, it needs to be abstracted.
		pi, err := p.ingestParamsFromRequest(r)
		if err != nil {
			p.writeInvalidParameterError(w, err)
			return
		}

		format := r.URL.Query().Get("format")
		contentType := r.Header.Get("Content-Type")
		pi.Val = tree.New()
		cb := pi.Val.InsertInt
		switch {
		case format == "trie", contentType == "binary/octet-stream+trie":
			tmpBuf := p.bufferPool.Get()
			defer p.bufferPool.Put(tmpBuf)
			err = transporttrie.IterateRaw(r.Body, tmpBuf.B, cb)
		case format == "tree", contentType == "binary/octet-stream+tree":
			err = convert.ParseTreeNoDict(r.Body, cb)
		case format == "lines":
			err = convert.ParseIndividualLines(r.Body, cb)
		default:
			err = convert.ParseGroups(r.Body, cb)
		}

		if err != nil {
			p.writeError(w, http.StatusUnprocessableEntity, err, "error happened while parsing request body")
			return
		}

		if err = p.storage.Put(pi); err != nil {
			p.writeInternalServerError(w, err, "error happened while ingesting data")
			return
		}
	})
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
	var cmd *exec.Cmd
	// Note that we don't specify which signals to be sent: any signal to be
	// relayed to the child process (including SIGINT and SIGTERM).
	signal.Notify(c)
	cmd = exec.Command(p.args[0], p.args[1:]...)
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

func (p push) ingestParamsFromRequest(r *http.Request) (*storage.PutInput, error) {
	var (
		q   = r.URL.Query()
		pi  storage.PutInput
		err error
	)

	pi.Key, err = segment.ParseKey(q.Get("name"))
	if err != nil {
		return nil, fmt.Errorf("name: %w", err)
	}

	if qt := q.Get("from"); qt != "" {
		pi.StartTime = attime.Parse(qt)
	} else {
		pi.StartTime = time.Now()
	}

	if qt := q.Get("until"); qt != "" {
		pi.EndTime = attime.Parse(qt)
	} else {
		pi.EndTime = time.Now()
	}

	if sr := q.Get("sampleRate"); sr != "" {
		sampleRate, err := strconv.Atoi(sr)
		if err != nil {
			p.logger.WithError(err).Errorf("invalid sample rate: %q", sr)
			pi.SampleRate = types.DefaultSampleRate
		} else {
			pi.SampleRate = uint32(sampleRate)
		}
	} else {
		pi.SampleRate = types.DefaultSampleRate
	}

	if sn := q.Get("spyName"); sn != "" {
		// TODO: error handling
		pi.SpyName = sn
	} else {
		pi.SpyName = "unknown"
	}

	if u := q.Get("units"); u != "" {
		pi.Units = u
	} else {
		pi.Units = "samples"
	}

	if at := q.Get("aggregationType"); at != "" {
		pi.AggregationType = at
	} else {
		pi.AggregationType = "sum"
	}

	return &pi, nil
}

func (p push) writeError(w http.ResponseWriter, code int, err error, msg string) {
	p.logger.WithError(err).Error(msg)
	writeMessage(w, code, "%s: %q", msg, err)
}

func (p push) writeInvalidMethodError(w http.ResponseWriter) {
	p.writeErrorMessage(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (p push) writeInvalidParameterError(w http.ResponseWriter, err error) {
	p.writeError(w, http.StatusBadRequest, err, "invalid parameter")
}

func (p push) writeInternalServerError(w http.ResponseWriter, err error, msg string) {
	p.writeError(w, http.StatusInternalServerError, err, msg)
}

func (p push) writeJSONEncodeError(w http.ResponseWriter, err error) {
	p.writeInternalServerError(w, err, "encoding response body")
}

func (p push) writeErrorMessage(w http.ResponseWriter, code int, msg string) {
	p.logger.Error(msg)
	writeMessage(w, code, msg)
}

func writeMessage(w http.ResponseWriter, code int, format string, args ...interface{}) {
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, format, args...)
	_, _ = fmt.Fprintln(w)
}
