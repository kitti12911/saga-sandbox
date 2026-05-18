package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	sagav1 "github.com/kitti12911/saga-sandbox/gen/grpc/saga/v1"
	"github.com/kitti12911/saga-sandbox/internal/server/interceptor"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

type GRPCServer struct {
	server   *grpc.Server
	listener net.Listener
	health   *health.Server
}

func NewGRPCServer(
	ctx context.Context,
	port int,
	sagaHandler sagav1.SagaServiceServer,
) (*GRPCServer, error) {
	listenConfig := net.ListenConfig{}
	listener, err := listenConfig.Listen(ctx, "tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	recoveryOpt := recovery.WithRecoveryHandler(func(p any) error {
		return status.Errorf(codes.Internal, "internal server error: %v", p)
	})

	srv := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler(
			otelgrpc.WithFilter(interceptor.TraceableRPC),
		)),
		grpc.ChainUnaryInterceptor(
			interceptor.ErrorHandler(),
			recovery.UnaryServerInterceptor(recoveryOpt),
		),
		grpc.ChainStreamInterceptor(
			recovery.StreamServerInterceptor(recoveryOpt),
		),
	)

	sagav1.RegisterSagaServiceServer(srv, sagaHandler)

	healthServer := health.NewServer()
	healthv1.RegisterHealthServer(srv, healthServer)
	healthServer.SetServingStatus("", healthv1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus(
		sagav1.SagaService_ServiceDesc.ServiceName,
		healthv1.HealthCheckResponse_SERVING,
	)

	reflection.Register(srv)

	return &GRPCServer{
		server:   srv,
		listener: listener,
		health:   healthServer,
	}, nil
}

func (s *GRPCServer) Start() error {
	slog.Info("gRPC server listening", "addr", s.listener.Addr().String())
	return s.server.Serve(s.listener)
}

func (s *GRPCServer) Stop(ctx context.Context) {
	s.health.Shutdown()

	done := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		slog.WarnContext(ctx, "graceful shutdown timed out, forcing stop")
		s.server.Stop()
	}
}
