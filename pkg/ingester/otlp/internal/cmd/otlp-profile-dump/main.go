package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	profilesv1 "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
)

type dumpServer struct {
	profilesv1.UnimplementedProfilesServiceServer
	outDir  string
	counter atomic.Int64
}

func (s *dumpServer) Export(ctx context.Context, req *profilesv1.ExportProfilesServiceRequest) (*profilesv1.ExportProfilesServiceResponse, error) {
	n := s.counter.Add(1)
	ts := time.Now().Format("20060102-150405")
	base := fmt.Sprintf("dump-%s-%04d", ts, n)

	binPath := filepath.Join(s.outDir, base+".pb.bin")
	jsonPath := filepath.Join(s.outDir, base+".pb.json")

	bin, err := proto.Marshal(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal binary: %v\n", err)
		return &profilesv1.ExportProfilesServiceResponse{}, nil
	}
	if err := os.WriteFile(binPath, bin, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", binPath, err)
		return &profilesv1.ExportProfilesServiceResponse{}, nil
	}

	jsonBytes, err := protojson.Marshal(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal json: %v\n", err)
		return &profilesv1.ExportProfilesServiceResponse{}, nil
	}
	if err := os.WriteFile(jsonPath, jsonBytes, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", jsonPath, err)
		return &profilesv1.ExportProfilesServiceResponse{}, nil
	}

	// Print summary with sample types
	nProfiles := 0
	dict := req.GetDictionary()
	var sampleTypes []string
	for _, rp := range req.GetResourceProfiles() {
		for _, sp := range rp.GetScopeProfiles() {
			nProfiles += len(sp.GetProfiles())
			for _, p := range sp.GetProfiles() {
				if dict != nil && p.GetSampleType() != nil {
					st := p.GetSampleType()
					strTable := dict.GetStringTable()
					typStr, unitStr := "", ""
					if int(st.TypeStrindex) < len(strTable) {
						typStr = strTable[st.TypeStrindex]
					}
					if int(st.UnitStrindex) < len(strTable) {
						unitStr = strTable[st.UnitStrindex]
					}
					sampleTypes = append(sampleTypes, typStr+":"+unitStr)
				}
			}
		}
	}
	fmt.Printf("[%04d] %s  %d bytes  %d profiles  types=%v\n", n, base, len(bin), nProfiles, sampleTypes)

	return &profilesv1.ExportProfilesServiceResponse{}, nil
}

func main() {
	outDir := ".tmp/otlp-dumps"
	listenAddr := ":11000"
	if len(os.Args) > 1 {
		listenAddr = os.Args[1]
	}
	if len(os.Args) > 2 {
		outDir = os.Args[2]
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", outDir, err)
		os.Exit(1)
	}

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen %s: %v\n", listenAddr, err)
		os.Exit(1)
	}

	srv := grpc.NewServer(
		grpc.MaxRecvMsgSize(64*1024*1024),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,
		}),
	)
	profilesv1.RegisterProfilesServiceServer(srv, &dumpServer{outDir: outDir})

	fmt.Printf("OTLP profile dump server listening on %s, writing to %s\n", listenAddr, outDir)
	if err := srv.Serve(lis); err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		os.Exit(1)
	}
}
