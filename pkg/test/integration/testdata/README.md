## Data generation

OTLP ingest handler has been updated to be compatible with OTLP protocol 1.8. Unfortunately, otel-collector does not yet have full support for 1.8.0, which prevents us from using otelcollector-contrib compiled image. Below procedure should be revisited once otel-collector is fully updated to 1.8.

To generate data for this fixture, use following procedure:

1. Patch ingest_handler.go to dump the received data:

```
diff --git a/pkg/ingester/otlp/ingest_handler.go b/pkg/ingester/otlp/ingest_handler.go
index bf7a6612f..1a6619243 100644
--- a/pkg/ingester/otlp/ingest_handler.go
+++ b/pkg/ingester/otlp/ingest_handler.go
@@ -2,13 +2,18 @@ package otlp
 
 import (
 	"context"
+	"encoding/json"
 	"fmt"
 	"net/http"
+	"os"
+	"path/filepath"
 	"strings"
+	"time"
 
 	"google.golang.org/grpc"
 	"google.golang.org/grpc/codes"
 	"google.golang.org/grpc/keepalive"
+	"google.golang.org/protobuf/encoding/protojson"
 	"google.golang.org/protobuf/proto"
 
 	"github.com/go-kit/log"
@@ -103,6 +108,20 @@ func (h *ingestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
 }
 
 func (h *ingestHandler) Export(ctx context.Context, er *pprofileotlp.ExportProfilesServiceRequest) (*pprofileotlp.ExportProfilesServiceResponse, error) {
+	// Debug: dump request to files & optionally corrupt data
+	//for _, rp := range er.GetResourceProfiles() {
+	//	for _, sp := range rp.GetScopeProfiles() {
+	//		for _, p := range sp.GetProfiles() {
+	//			for _, s := range p.Sample {
+	//				s.StackIndex = 1000000000
+	//			}
+	//		}
+	//	}
+	//}
+	if err := h.dumpRequestToFiles(er); err != nil {
+		level.Warn(h.log).Log("msg", "failed to dump request to debug files", "err", err)
+	}
+
 	// TODO: @petethepig This logic is copied from util.AuthenticateUser and should be refactored into a common function
 	// Extracts user ID from the request metadata and returns and injects the user ID in the context
 	if !h.multitenancyEnabled {
@@ -243,3 +262,55 @@ func appendAttributesUnique(labels []*typesv1.LabelPair, attrs []*v1.KeyValue, p
 	}
 	return labels
 }
+
+// dumpRequestToFiles dumps the received request to both protobuf and JSON formats for debugging
+func (h *ingestHandler) dumpRequestToFiles(er *pprofileotlp.ExportProfilesServiceRequest) error {
+	captureDir := "/tmp/capture"
+
+	// Ensure capture directory exists
+	if err := os.MkdirAll(captureDir, 0755); err != nil {
+		return fmt.Errorf("failed to create capture directory: %w", err)
+	}
+
+	// Generate timestamp-based filename
+	timestamp := time.Now().UnixNano()
+
+	// Dump as protobuf binary
+	pbData, err := proto.Marshal(er)
+	if err != nil {
+		return fmt.Errorf("failed to marshal protobuf: %w", err)
+	}
+
+	pbFilename := filepath.Join(captureDir, fmt.Sprintf("%d.pb.bin", timestamp))
+	if err := os.WriteFile(pbFilename, pbData, 0644); err != nil {
+		return fmt.Errorf("failed to write protobuf file: %w", err)
+	}
+
+	// Dump as formatted JSON
+	jsonData, err := protojson.MarshalOptions{
+		Multiline: true,
+		Indent:    "  ",
+	}.Marshal(er)
+	if err != nil {
+		return fmt.Errorf("failed to marshal JSON: %w", err)
+	}
+
+	// Pretty print JSON
+	var prettyJSON map[string]interface{}
+	if err := json.Unmarshal(jsonData, &prettyJSON); err != nil {
+		return fmt.Errorf("failed to unmarshal JSON for pretty printing: %w", err)
+	}
+
+	formattedJSON, err := json.MarshalIndent(prettyJSON, "", "  ")
+	if err != nil {
+		return fmt.Errorf("failed to marshal pretty JSON: %w", err)
+	}
+
+	jsonFilename := filepath.Join(captureDir, fmt.Sprintf("%d.pb.json", timestamp))
+	if err := os.WriteFile(jsonFilename, formattedJSON, 0644); err != nil {
+		return fmt.Errorf("failed to write JSON file: %w", err)
+	}
+
+	level.Debug(h.log).Log("msg", "dumped request to debug files", "pb_file", pbFilename, "json_file", jsonFilename)
+	return nil
+}
```
2.Launch local pyroscope instance
3.Compile & start ebpf profiler with following parameters:

```
./ebpf-profiler -collection-agent <pyroscope:4040> -off-cpu-threshold 1 -disable-tls
```

Example command to start profiler under Docker/Podman on Mac (from a source directory with compiled profiler):
```
podman run -v "$PWD":/agent --mount type=bind,source=/sys/kernel/tracing,target=/sys/kernel/tracing --mount type=bind,source=/sys/kernel/debug,target=/sys/kernel/debug -it --privileged --replace --pid=host --name ebpf --user 0:0 otel/opentelemetry-ebpf-profiler-dev:latest /agent/ebpf-profiler -collection-agent <pyroscope:4040> -off-cpu-threshold 1 -disable-tls
```
Note that this will capture live data from all processes on the machine it runs on

4.Allow some profiles to be gathered, explore /tmp/capture dir
