package heapanalyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/grafana/dskit/services"
	"golang.org/x/exp/slices"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/gocore"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

type HeapAnalyzer struct {
	services.Service
	localDir string
	logger   log.Logger

	dumpsSync sync.Mutex       // for now only one access at a time TODO improve it
	dumps     map[string]*Dump // TODO do not grow indefinitely, need cleanup/lru
}

func NewHeapAnalyzer(logger log.Logger) *HeapAnalyzer {
	h := &HeapAnalyzer{
		logger:   logger,
		localDir: "/tmp/heapdumps", // todo configure
		dumps:    map[string]*Dump{},
	}
	h.Service = services.NewBasicService(nil, h.running, nil)
	err := os.MkdirAll(h.localDir, 0o755)
	if err != nil {
		panic(err)
	}
	return h
}

func (h *HeapAnalyzer) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

const heapDumpInfoFile = "info.json"

// ingest file pyro.core.71993 and ___2pyroscope
// curl  -F core='@pyro.core.71993' -F exe='@___2pyroscope' -F labels='{"namespace":"foo", "pod":"bar"}' -X POST http://localhost:4040/heap-analyzer/ingest
func (h *HeapAnalyzer) IngestHandler(w http.ResponseWriter, r *http.Request) {
	id := uuid.New().String()
	level.Info(h.logger).Log("msg", "ingesting heap dump", "id", id)
	fr, err := r.MultipartReader()
	if err != nil {
		httputil.Error(w, err)
		return
	}
	heapDump := &HeapDump{
		Id:        id,
		CreatedAt: time.Now().UnixMilli(),
		Labels:    &typesv1.Labels{},
	}
	for {
		part, err := fr.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			httputil.Error(w, err)
			return
		}
		name := part.FormName()
		switch name {
		case "core", "exe":
			err = writeDumpFile(h.localDir, heapDump.Id, name, part)
			if err != nil {
				httputil.Error(w, err)
				return
			}
		case "labels":
			ls := map[string]string{}
			err = json.NewDecoder(part).Decode(&ls)
			if err != nil {
				httputil.Error(w, err)
				return
			}
			for k, v := range ls {
				heapDump.Labels.Labels = append(heapDump.Labels.Labels, &typesv1.LabelPair{Name: k, Value: v})
			}
			slices.SortFunc(heapDump.Labels.Labels, func(i, j *typesv1.LabelPair) int {
				return strings.Compare(i.Name, j.Name)
			})
		default:
			httputil.Error(w, fmt.Errorf("unknown part: %s", name))
			return
		}
	}
	heapDumpBytes, err := json.Marshal(heapDump)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	err = writeDumpFile(h.localDir, heapDump.Id, heapDumpInfoFile, bytes.NewReader(heapDumpBytes))
	if err != nil {
		httputil.Error(w, err)
		return
	}
}

// curl   http://localhost:4040/heap-analyzer/heap-dumps
func (h *HeapAnalyzer) HeapDumpsHandler(w http.ResponseWriter, r *http.Request) {
	var heapDumps []*HeapDump
	dumps, err := os.ReadDir(h.localDir)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	for _, d := range dumps {
		if !d.IsDir() {
			continue
		}
		id, err := uuid.Parse(d.Name())
		if err != nil {
			level.Error(h.logger).Log("msg", "error parsing heap dump id", "id", d.Name(), "err", err)
			continue
		}
		heapDump, err := h.readHeapDumpInfo(id.String())
		if err != nil {
			level.Error(h.logger).Log("msg", "error reading heap dump info", "id", d.Name(), "err", err)
			continue
		}
		heapDumps = append(heapDumps, heapDump)
	}
	data, err := json.Marshal(heapDumps)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		httputil.Error(w, err)
	}
}

// curl   http://localhost:4040/heap-analyzer/heap-dump/0eed7d49-b9da-420d-b4a4-f041b2aca70b
func (h *HeapAnalyzer) HeapDumpHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getHeapDumpId(r)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	level.Info(h.logger).Log("msg", "retrieving heap dump", "hid", id)
	info, err := h.readHeapDumpInfo(id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	data, err := json.Marshal(info)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		httputil.Error(w, err)
	}
}

// curl   http://localhost:4040/heap-analyzer/heap-dump/0eed7d49-b9da-420d-b4a4-f041b2aca70b/object-types
func (h *HeapAnalyzer) HeapDumpObjectTypesHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getHeapDumpId(r)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	level.Info(h.logger).Log("msg", "retrieving heap dump object types", "hid", id)

	h.dumpsSync.Lock()
	defer h.dumpsSync.Unlock()

	dump, err := h.getDumpLocked(id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	types := dump.ObjectTypes()
	data, err := json.Marshal(types)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		httputil.Error(w, err)
	}
}

// curl   http://localhost:4040/heap-analyzer/heap-dump/0eed7d49-b9da-420d-b4a4-f041b2aca70b/objects
// curl  "http://localhost:4040/heap-analyzer/heap-dump/0eed7d49-b9da-420d-b4a4-f041b2aca70b/objects?type_re=^net/http.*\$&offset=10&limit=10" | jq
// curl "http://localhost:4040/heap-analyzer/heap-dump/0eed7d49-b9da-420d-b4a4-f041b2aca70b/objects?type=net/http.Request&offset=10&limit=10" | jq
func (h *HeapAnalyzer) HeapDumpObjectsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getHeapDumpId(r)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	level.Info(h.logger).Log("msg", "retrieving heap dump objects", "hid", id)

	h.dumpsSync.Lock()
	defer h.dumpsSync.Unlock()

	dump, err := h.getDumpLocked(id)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	var filter Filter[*Object] = NoFilter[*Object]{}
	if r.URL.Query().Get("type") != "" {
		filter = ObjectTypeNameFilter{r.URL.Query().Get("type")}
	} else if r.URL.Query().Get("type_re") != "" {
		re, err := regexp.Compile(r.URL.Query().Get("type_re"))
		if err != nil {
			httputil.Error(w, err)
			return
		}
		filter = ObjectTypeNameRegexFilter{re}
	}

	result := dump.ObjectsFilter(filter)
	result.Items = pagination(result.Items, r)

	data, err := json.Marshal(result)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		httputil.Error(w, err)
	}
}

func (h *HeapAnalyzer) HeapDumpObjectHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getHeapDumpId(r)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	objID, err := getObjectId(r)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	h.dumpsSync.Lock()
	defer h.dumpsSync.Unlock()

	dump, err := h.getDumpLocked(id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	obj, err := dump.findObject(objID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	fields, err := dump.ObjectFields(objID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	references, err := dump.ObjectReferences(objID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	tName := typeName(dump.gocore, obj.obj)
	objectWithDetails := &ObjectWithDetails{
		Object: Object{
			Id:          fmt.Sprintf("%x", obj.addr),
			Type:        tName,
			Address:     fmt.Sprintf("%x", obj.addr),
			DisplayName: fmt.Sprintf("%s [%x]", tName, obj.addr),
			Size:        obj.size,
		},
		Fields:     fields,
		References: references,
	}

	data, err := json.Marshal(objectWithDetails)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		httputil.Error(w, err)
	}

	level.Info(h.logger).Log("msg", "retrieving heap dump object details", "hid", id, "oid", objID)
}

func (h *HeapAnalyzer) HeapObjectReachableHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getHeapDumpId(r)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	objID, err := getObjectId(r)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	h.dumpsSync.Lock()
	defer h.dumpsSync.Unlock()

	dump, err := h.getDumpLocked(id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	reachable, err := dump.ObjectReachable(objID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	data, err := json.Marshal(reachable)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		httputil.Error(w, err)
	}

	level.Info(h.logger).Log("msg", "retrieving object reachable from root", "hid", id, "oid", objID)
}

func (h *HeapAnalyzer) readHeapDumpInfo(id string) (*HeapDump, error) {
	heapDumpBytes, err := os.ReadFile(h.localDir + "/" + id + "/" + heapDumpInfoFile)
	if err != nil {
		return nil, err
	}
	heapDump := new(HeapDump)
	err = json.Unmarshal(heapDumpBytes, heapDump)
	if err != nil {
		return nil, err
	}
	return heapDump, nil
}

func (h *HeapAnalyzer) getDumpLocked(id string) (*Dump, error) {
	d, ok := h.dumps[id]
	if ok {
		return d, nil
	}
	heapDump, err := h.readHeapDumpInfo(id)
	if err != nil {
		return nil, err
	}
	t1 := time.Now()
	d, err = NewDump(h.logger,
		h.localDir+"/"+id+"/exe",
		h.localDir+"/"+id+"/core",
		heapDump)
	t2 := time.Now()
	level.Info(h.logger).Log("msg", "NewDump", "id", id, "duration", t2.Sub(t1))
	if err != nil {
		return nil, err
	}
	h.dumps[id] = d
	return d, nil
}

func (h *HeapAnalyzer) HeapDumpInspectionsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getHeapDumpId(r)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	h.dumpsSync.Lock()
	defer h.dumpsSync.Unlock()

	dump, err := h.getDumpLocked(id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	inspectors := []inspector{
		NewDuplicateStringInspector(dump.gocore),
	}

	dump.gocore.ForEachObject(func(x gocore.Object) bool {
		for _, i := range inspectors {
			err := i.Consume(x)
			if err != nil {
				return false
			}
		}
		return true
	})

	results := make([]*InspectionResult, len(inspectors))
	for i, ins := range inspectors {
		results[i], err = ins.GetResult()
	}

	data, err := json.Marshal(results)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		httputil.Error(w, err)
	}
}

func writeDumpFile(dir string, id string, name string, part io.Reader) error {
	fname := dir + "/" + id + "/" + name
	err := os.MkdirAll(filepath.Dir(fname), 0o755)
	if err != nil {
		return err
	}
	f, err := os.Create(fname)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, part)
	if err != nil {
		return err
	}
	return nil
}

func getHeapDumpId(r *http.Request) (string, error) {
	vars := mux.Vars(r)
	id := vars["id"]
	_, err := uuid.Parse(id)
	if err != nil {
		return "", fmt.Errorf("invalid heap dump id: %w", err)
	}
	return id, nil
}

func getObjectId(r *http.Request) (int64, error) {
	vars := mux.Vars(r)

	id := vars["oid"]
	obj, err := strconv.ParseInt(id, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid oif %s", err)
	}

	return obj, nil
}

func getObjectFieldId(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["fid"]
}

func pagination[T any](ts []T, r *http.Request) []T {
	offset := 0
	limit := len(ts)
	if r.URL.Query().Get("offset") != "" {
		offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	}
	if r.URL.Query().Get("limit") != "" {
		limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	}
	if offset > len(ts) {
		offset = len(ts)
	}
	ts = ts[offset:]
	if limit < len(ts) {
		ts = ts[:limit]
	}
	return ts
}
