package httpgrpc

import (
	"github.com/grafana/dskit/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

func NewGrpcServer(cfg server.Config) *grpc.Server {
	grpcKeepAliveOptions := keepalive.ServerParameters{
		MaxConnectionIdle:     cfg.GRPCServerMaxConnectionIdle,
		MaxConnectionAge:      cfg.GRPCServerMaxConnectionAge,
		MaxConnectionAgeGrace: cfg.GRPCServerMaxConnectionAgeGrace,
		Time:                  cfg.GRPCServerTime,
		Timeout:               cfg.GRPCServerTimeout,
	}

	grpcKeepAliveEnforcementPolicy := keepalive.EnforcementPolicy{
		MinTime:             cfg.GRPCServerMinTimeBetweenPings,
		PermitWithoutStream: cfg.GRPCServerPingWithoutStreamAllowed,
	}

	grpcOptions := []grpc.ServerOption{
		grpc.KeepaliveParams(grpcKeepAliveOptions),
		grpc.KeepaliveEnforcementPolicy(grpcKeepAliveEnforcementPolicy),
		grpc.MaxRecvMsgSize(cfg.GRPCServerMaxRecvMsgSize),
		grpc.MaxSendMsgSize(cfg.GRPCServerMaxSendMsgSize),
		grpc.MaxConcurrentStreams(uint32(cfg.GRPCServerMaxConcurrentStreams)),
		grpc.NumStreamWorkers(uint32(cfg.GRPCServerNumWorkers)),
	}

	grpcOptions = append(grpcOptions, cfg.GRPCOptions...)

	return grpc.NewServer(grpcOptions...)
}
