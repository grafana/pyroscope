package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
	"github.com/sirupsen/logrus"
)

// RenderDiffParams refers to the params accepted by the renderDiffHandler
type RenderDiffParams struct {
	LeftQuery string `json:"leftQuery"`
	LeftFrom  string `json:"leftFrom"`
	LeftUntil string `json:"leftUntil"`

	RightQuery string `json:"rightQuery"`
	RightFrom  string `json:"rightFrom"`
	RightUntil string `json:"rightUntil"`

	Format   string `json:"format"`
	MaxNodes *int   `json:"maxNodes,omitempty"`
}

// RenderDiffResponse refers to the response of the renderDiffHandler
type RenderDiffResponse struct {
	*flamebearer.FlamebearerProfile
}

type diffParams struct {
	Left  storage.GetInput
	Right storage.GetInput

	Format   string
	MaxNodes int
}

// parseDiffQueryParams parses query params into a diffParams
func (rh *RenderDiffHandler) parseDiffQueryParams(r *http.Request, p *diffParams) (err error) {
	parseDiffQueryParams := func(r *http.Request, prefix string) (gi storage.GetInput, err error) {
		v := r.URL.Query()
		getWithPrefix := func(param string) string {
			return v.Get(prefix + strings.Title(param))
		}

		// Parse query
		qry, err := flameql.ParseQuery(getWithPrefix("query"))
		if err != nil {
			return gi, fmt.Errorf("%q: %+w", "Error parsing query", err)
		}
		gi.Query = qry

		gi.StartTime = attime.Parse(getWithPrefix("from"))
		gi.EndTime = attime.Parse(getWithPrefix("until"))

		return gi, nil
	}

	p.Left, err = parseDiffQueryParams(r, "left")
	if err != nil {
		return fmt.Errorf("%q: %+w", "Could not parse 'left' side", err)
	}

	p.Right, err = parseDiffQueryParams(r, "right")
	if err != nil {
		return fmt.Errorf("%q: %+w", "Could not parse 'right' side", err)
	}

	// Parse the common fields
	v := r.URL.Query()
	p.MaxNodes = rh.maxNodesDefault
	if mn, err := strconv.Atoi(v.Get("max-nodes")); err == nil && mn > 0 {
		p.MaxNodes = mn
	}

	p.Format = v.Get("format")
	return expectFormats(p.Format)
}

func (ctrl *Controller) renderDiffHandler() http.Handler {
	return NewRenderDiffHandler(ctrl.log, ctrl.storage, ctrl.dir, ctrl, ctrl.config.MaxNodesRender)
}

type RenderDiffHandler struct {
	log             *logrus.Logger
	storage         storage.Getter
	dir             http.FileSystem
	stats           StatsReceiver
	maxNodesDefault int
}

func NewRenderDiffHandler(l *logrus.Logger, s storage.Getter, dir http.FileSystem, stats StatsReceiver, maxNodesDefault int) *RenderDiffHandler {
	return &RenderDiffHandler{
		log:             l,
		storage:         s,
		dir:             dir,
		stats:           stats,
		maxNodesDefault: maxNodesDefault,
	}
}

func (rh *RenderDiffHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var params diffParams

	switch r.Method {
	case http.MethodGet:
		if err := rh.parseDiffQueryParams(r, &params); err != nil {
			rh.writeInvalidParameterError(w, err)
			return
		}
	default:
		rh.writeInvalidMethodError(w)
		return
	}

	// Load Both trees
	// TODO: do this concurrently
	leftOut, err := rh.loadTree(&params.Left, params.Left.StartTime, params.Left.EndTime)
	if err != nil {
		rh.writeInvalidParameterError(w, fmt.Errorf("%q: %+w", "could not load 'left' tree", err))
		return
	}

	rightOut, err := rh.loadTree(&params.Right, params.Right.StartTime, params.Right.EndTime)
	if err != nil {
		rh.writeInvalidParameterError(w, fmt.Errorf("%q: %+w", "could not load 'right' tree", err))
		return
	}

	combined, err := flamebearer.NewCombinedProfile("diff", leftOut, rightOut, params.MaxNodes)
	if err != nil {
		rh.writeInvalidParameterError(w, err)
		return
	}

	switch params.Format {
	case "html":
		w.Header().Add("Content-Type", "text/html")
		if err := flamebearer.FlamebearerToStandaloneHTML(&combined, rh.dir, w); err != nil {
			rh.writeJSONEncodeError(w, err)
			return
		}

	case "json":
		// fallthrough to default, to maintain existing behaviour
		fallthrough
	default:
		res := RenderDiffResponse{&combined}
		rh.writeResponseJSON(w, res)
	}
}

//revive:disable-next-line:argument-limit 7 parameters here is fine
func (rh *RenderDiffHandler) loadTreeConcurrently(
	gi *storage.GetInput,
	treeStartTime, treeEndTime time.Time,
	leftStartTime, leftEndTime time.Time,
	rghtStartTime, rghtEndTime time.Time,
) (treeOut, leftOut, rghtOut *storage.GetOutput, _ error) {
	var treeErr, leftErr, rghtErr error
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); treeOut, treeErr = rh.loadTree(gi, treeStartTime, treeEndTime) }()
	go func() { defer wg.Done(); leftOut, leftErr = rh.loadTree(gi, leftStartTime, leftEndTime) }()
	go func() { defer wg.Done(); rghtOut, rghtErr = rh.loadTree(gi, rghtStartTime, rghtEndTime) }()
	wg.Wait()

	for _, err := range []error{treeErr, leftErr, rghtErr} {
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return treeOut, leftOut, rghtOut, nil
}

func (rh *RenderDiffHandler) loadTree(gi *storage.GetInput, startTime, endTime time.Time) (_ *storage.GetOutput, _err error) {
	defer func() {
		rerr := recover()
		if rerr != nil {
			_err = fmt.Errorf("panic: %v", rerr)
			rh.log.WithFields(logrus.Fields{
				"recover": rerr,
				"stack":   string(debug.Stack()),
			}).Error("loadTree: recovered from panic")
		}
	}()

	_gi := *gi // clone the struct
	_gi.StartTime, _gi.EndTime = startTime, endTime
	out, err := rh.storage.Get(&_gi)
	if err != nil {
		return nil, err
	}
	if out == nil {
		// TODO: handle properly
		return &storage.GetOutput{Tree: tree.New()}, nil
	}
	return out, nil
}

// TODO: remove this

func (rh *RenderDiffHandler) writeResponseJSON(w http.ResponseWriter, res interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		rh.writeJSONEncodeError(w, err)
	}
}

func (*RenderDiffHandler) writeResponseFile(w http.ResponseWriter, filename string, content []byte) {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%v", filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(content)
	w.(http.Flusher).Flush()
}

func (rh *RenderDiffHandler) writeError(w http.ResponseWriter, code int, err error, msg string) {
	WriteError(rh.log, w, code, err, msg)
}

func (rh *RenderDiffHandler) writeInvalidMethodError(w http.ResponseWriter) {
	WriteErrorMessage(rh.log, w, http.StatusMethodNotAllowed, "method not allowed")
}

func (rh *RenderDiffHandler) writeInvalidParameterError(w http.ResponseWriter, err error) {
	rh.writeError(w, http.StatusBadRequest, err, "invalid parameter")
}

func (rh *RenderDiffHandler) writeInternalServerError(w http.ResponseWriter, err error, msg string) {
	rh.writeError(w, http.StatusInternalServerError, err, msg)
}

func (rh *RenderDiffHandler) writeJSONEncodeError(w http.ResponseWriter, err error) {
	rh.writeInternalServerError(w, err, "encoding response body")
}

func (rh *RenderDiffHandler) writeErrorMessage(w http.ResponseWriter, code int, msg string) {
	WriteErrorMessage(rh.log, w, code, msg)
}
