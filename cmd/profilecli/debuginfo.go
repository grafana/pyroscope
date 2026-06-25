package main

import (
	"context"
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"

	timestamppb "google.golang.org/protobuf/types/known/timestamppb"

	debuginfov1alpha1 "github.com/grafana/pyroscope/api/gen/proto/go/debuginfo/v1alpha1"
	debuginfov1alpha1connect "github.com/grafana/pyroscope/api/gen/proto/go/debuginfo/v1alpha1/debuginfov1alpha1connect"
	connectapi "github.com/grafana/pyroscope/v2/pkg/api/connect"
	"github.com/grafana/pyroscope/v2/pkg/debuginfo"
)

func (c *phlareClient) debuginfoServiceClient() debuginfov1alpha1connect.DebuginfoServiceClient {
	return debuginfov1alpha1connect.NewDebuginfoServiceClient(
		c.httpClient(),
		c.URL,
		append(
			connectapi.DefaultClientOptions(),
			c.protocolOption(),
		)...,
	)
}

// extractGnuBuildIdFromReader parses the GNU build ID out of an already-open
// ELF file. The caller is responsible for closing/seeking the reader.
func extractGnuBuildIdFromReader(r io.ReaderAt) (string, error) {
	elfFile, err := elf.NewFile(r)
	if err != nil {
		return "", err
	}
	defer elfFile.Close()

	for _, section := range elfFile.Sections {
		if section.Name != ".note.gnu.build-id" {
			continue
		}
		data, err := section.Data()
		if err != nil {
			return "", err
		}
		if len(data) < 12 {
			return "", fmt.Errorf(".note.gnu.build-id section too short: %d bytes", len(data))
		}
		namesz := binary.LittleEndian.Uint32(data[0:4])
		descsz := binary.LittleEndian.Uint32(data[4:8])
		nameEnd := 12 + int(namesz)
		descStart := (nameEnd + 3) &^ 3 // align to 4 bytes
		descEnd := descStart + int(descsz)
		if descStart < nameEnd || descEnd > len(data) {
			return "", fmt.Errorf(".note.gnu.build-id section truncated: need %d bytes, have %d", descEnd, len(data))
		}
		return hex.EncodeToString(data[descStart:descEnd]), nil
	}
	return "", nil
}

// extractGnuBuildId opens path and returns the file's GNU build ID.
func extractGnuBuildId(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return extractGnuBuildIdFromReader(f)
}

func shouldInitiateUploadCheck(ctx context.Context, client debuginfov1alpha1connect.DebuginfoServiceClient, gnuBuildId string, fileName string, fileType debuginfov1alpha1.FileMetadata_Type) (bool, string, error) {
	req := &debuginfov1alpha1.ShouldInitiateUploadRequest{
		File: &debuginfov1alpha1.FileMetadata{
			GnuBuildId: gnuBuildId,
			Name:       fileName,
			Type:       fileType,
		},
	}
	resp, err := client.ShouldInitiateUpload(ctx, connect.NewRequest(req))
	if err != nil {
		return false, "", err
	}
	return resp.Msg.ShouldInitiateUpload, resp.Msg.Reason, nil
}

func uploadDebuginfo(ctx context.Context, params *debuginfoUploadParams) error {
	client := params.debuginfoServiceClient()

	f, err := os.Open(params.path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	gnuBuildId, err := extractGnuBuildIdFromReader(f)
	if err != nil {
		return fmt.Errorf("failed to extract GNU build ID from %q: %w", params.path, err)
	}
	if gnuBuildId == "" {
		return fmt.Errorf("file %q has no .note.gnu.build-id section; cannot upload", params.path)
	}

	var fileType debuginfov1alpha1.FileMetadata_Type
	switch params.fileType {
	case "executable-full":
		fileType = debuginfov1alpha1.FileMetadata_TYPE_EXECUTABLE_FULL
	case "executable-no-text":
		fileType = debuginfov1alpha1.FileMetadata_TYPE_EXECUTABLE_NO_TEXT
	default:
		fileType = debuginfov1alpha1.FileMetadata_TYPE_UNSPECIFIED
	}

	shouldUpload, reason, err := shouldInitiateUploadCheck(ctx, client, gnuBuildId, filepath.Base(params.path), fileType)
	if err != nil {
		return fmt.Errorf("ShouldInitiateUpload check failed: %w", err)
	}
	if !shouldUpload {
		if reason == debuginfo.ReasonDisabled {
			return fmt.Errorf("server has debuginfo upload disabled")
		}
		level.Info(logger).Log("msg", "server declined upload", "build_id", gnuBuildId, "reason", reason)
		return nil
	}

	// Rewind so the upload reads the file from the start.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to rewind file: %w", err)
	}

	uploadURL := params.URL + "/debuginfo.v1alpha1.DebuginfoService/Upload/" + gnuBuildId
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, f)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}

	resp, err := params.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload failed with status %s", resp.Status)
	}

	if _, err := client.UploadFinished(ctx, connect.NewRequest(&debuginfov1alpha1.UploadFinishedRequest{
		GnuBuildId: gnuBuildId,
	})); err != nil {
		return fmt.Errorf("failed to finish upload: %w", err)
	}

	level.Info(logger).Log("msg", "successfully uploaded debuginfo", "build_id", gnuBuildId, "path", params.path)
	return nil
}

type debuginfoUploadParams struct {
	path     string
	fileType string
	*phlareClient
}

func addDebuginfoUploadParams(cmd commander) *debuginfoUploadParams {
	params := new(debuginfoUploadParams)
	cmd.Arg("path", "Path to the file to upload").Required().ExistingFileVar(&params.path)
	cmd.Flag("type", "Type of executable: executable-full, executable-no-text").Default("executable-full").StringVar(&params.fileType)

	params.phlareClient = addPhlareClient(cmd)
	return params
}

type debuginfoListParams struct {
	*phlareClient
}

func addDebuginfoListParams(cmd commander) *debuginfoListParams {
	params := new(debuginfoListParams)
	params.phlareClient = addPhlareClient(cmd)
	return params
}

func listDebuginfo(ctx context.Context, params *debuginfoListParams) error {
	client := params.debuginfoServiceClient()

	req := &debuginfov1alpha1.ListDebuginfoRequest{}

	resp, err := client.ListDebuginfo(ctx, connect.NewRequest(req))
	if err != nil {
		return fmt.Errorf("failed to list debuginfo: %w", err)
	}

	for _, object := range resp.Msg.GetObject() {
		file := object.GetFile()
		fmt.Printf("build_id=%s name=%s type=%s state=%s size=%s uploaded_at=%s\n",
			file.GetGnuBuildId(),
			file.GetName(),
			file.GetType().String(),
			object.GetState().String(),
			humanizeBytes(object.GetSizeBytes()),
			formatUploadedAt(object.GetFinishedAt()),
		)
	}

	return nil
}

type debuginfoDeleteParams struct {
	gnuBuildID string
	*phlareClient
}

func addDebuginfoDeleteParams(cmd commander) *debuginfoDeleteParams {
	params := new(debuginfoDeleteParams)
	cmd.Arg("gnu-build-id", "GNU build ID to delete").Required().StringVar(&params.gnuBuildID)
	params.phlareClient = addPhlareClient(cmd)
	return params
}

func deleteDebuginfo(ctx context.Context, params *debuginfoDeleteParams) error {
	client := params.debuginfoServiceClient()

	_, err := client.DeleteDebuginfo(ctx, connect.NewRequest(&debuginfov1alpha1.DeleteDebuginfoRequest{
		GnuBuildId: params.gnuBuildID,
	}))
	if err != nil {
		return fmt.Errorf("failed to delete debuginfo: %w", err)
	}

	fmt.Printf("deleted debuginfo build_id=%s\n", params.gnuBuildID)
	return nil
}

func formatUploadedAt(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return "in-progress"
	}
	return ts.AsTime().Format("2006-01-02T15:04:05Z")
}

const unknownSize = "unknown"

func humanizeBytes(b int64) string {
	if b == 0 {
		return unknownSize
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
