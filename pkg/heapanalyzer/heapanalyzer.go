package heapanalyzer

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/mux"
	"github.com/grafana/dskit/services"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

type HeapAnalyzer struct {
	services.Service

	logger log.Logger
}

func NewHeapAnalyzer(logger log.Logger) *HeapAnalyzer {
	h := &HeapAnalyzer{
		logger: logger,
	}
	h.Service = services.NewBasicService(nil, h.running, nil)
	return h
}

func (h *HeapAnalyzer) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

var (
	dummyHeapDump = HeapDump{
		Id:        "dummy",
		CreatedAt: time.Now().UnixMilli(),
		Labels: &typesv1.Labels{
			Labels: []*typesv1.LabelPair{
				{Name: "pod", Value: "fire-ingester-0"},
				{Name: "namespace", Value: "fire-dev-001"},
			},
		},
	}
)

func (h *HeapAnalyzer) HeapDumpsHandler(w http.ResponseWriter, r *http.Request) {
	var heapDumps = []*HeapDump{&dummyHeapDump}
	data, _ := json.Marshal(heapDumps)
	_, err := w.Write(data)
	if err != nil {
		httputil.Error(w, err)
	}
}

func (h *HeapAnalyzer) HeapDumpHandler(w http.ResponseWriter, r *http.Request) {
	data, _ := json.Marshal(dummyHeapDump)
	_, err := w.Write(data)
	if err != nil {
		httputil.Error(w, err)
	}
	level.Info(h.logger).Log("msg", "retrieving heap dump", "hid", getHeapDumpId(r))
}

func (h *HeapAnalyzer) HeapDumpObjectTypesHandler(w http.ResponseWriter, r *http.Request) {
	level.Info(h.logger).Log("msg", "retrieving heap dump object types", "hid", getHeapDumpId(r))
}

func (h *HeapAnalyzer) HeapDumpObjectsHandler(w http.ResponseWriter, r *http.Request) {
	level.Info(h.logger).Log("msg", "retrieving heap dump objects", "hid", getHeapDumpId(r))
}

func (h *HeapAnalyzer) HeapDumpObjectHandler(w http.ResponseWriter, r *http.Request) {
	level.Info(h.logger).Log("msg", "retrieving heap dump object", "hid", getHeapDumpId(r), "oid", getObjectId(r))
}

func (h *HeapAnalyzer) HeapDumpObjectReferencesHandler(w http.ResponseWriter, r *http.Request) {
	level.Info(h.logger).Log("msg", "retrieving heap dump object references", "hid", getHeapDumpId(r), "oid", getObjectId(r))
}

func (h *HeapAnalyzer) HeapDumpObjectFieldsHandler(w http.ResponseWriter, r *http.Request) {
	level.Info(h.logger).Log("msg", "retrieving heap dump object fields", "hid", getHeapDumpId(r), "oid", getObjectId(r))
}

func getHeapDumpId(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["id"]
}

func getObjectId(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["oid"]
}

func getObjectFieldId(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["fid"]
}
