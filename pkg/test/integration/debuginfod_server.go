package integration

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type TestDebuginfodServer struct {
	server     *http.Server
	debugFiles map[string]string
	listener   net.Listener
}

func NewTestDebuginfodServer() (*TestDebuginfodServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}

	s := &TestDebuginfodServer{
		debugFiles: make(map[string]string),
		listener:   listener,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/buildid/", s.handleBuildID)

	s.server = &http.Server{
		Handler: mux,
	}

	return s, nil
}

func (s *TestDebuginfodServer) AddDebugFile(buildID, filePath string) {
	s.debugFiles[buildID] = filePath
}

func (s *TestDebuginfodServer) URL() string {
	return fmt.Sprintf("http://%s", s.listener.Addr().String())
}

func (s *TestDebuginfodServer) Start() error {
	go func() {
		_ = s.server.Serve(s.listener)
	}()
	return nil
}

func (s *TestDebuginfodServer) Stop() error {
	return s.server.Close()
}

func (s *TestDebuginfodServer) handleBuildID(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/buildid/"), "/")
	if len(parts) != 2 || parts[1] != "debuginfo" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	buildID := parts[0]
	filePath, ok := s.debugFiles[buildID]
	if !ok {
		http.Error(w, "Build ID not found", http.StatusNotFound)
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to open debug file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to stat debug file: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(filePath)))

	_, err = io.Copy(w, file)
	if err != nil {
		return
	}
}
