package interceptor

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/kitti12911/lib-util/v3/apperror"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

var codeMap = map[apperror.Code]codes.Code{
	apperror.CodeInternal:     codes.Internal,
	apperror.CodeNotFound:     codes.NotFound,
	apperror.CodeAlreadyExist: codes.AlreadyExists,
	apperror.CodeInvalidInput: codes.InvalidArgument,
	apperror.CodeUnauthorized: codes.Unauthenticated,
	apperror.CodeForbidden:    codes.PermissionDenied,
}

func TraceableRPC(info *stats.RPCTagInfo) bool {
	if info == nil {
		return true
	}

	return !IsHealthCheck(info.FullMethodName)
}

func IsHealthCheck(fullMethod string) bool {
	return strings.HasPrefix(fullMethod, "/grpc.health.v1.Health/")
}

func ErrorHandler() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		resp, err := handler(ctx, req)
		if err == nil || (info != nil && IsHealthCheck(info.FullMethod)) {
			return resp, nil
		}

		traceID := extractTraceID(ctx)
		if traceID != "" {
			if trailerErr := grpc.SetTrailer(ctx, metadata.Pairs("x-trace-id", traceID)); trailerErr != nil {
				slog.WarnContext(ctx, "failed to set trace_id trailer", "error", trailerErr)
			}
		}

		if st, ok := status.FromError(err); ok {
			return nil, status.Error(st.Code(), messageWithTraceID(st.Message(), traceID))
		}

		logAttrs := []any{
			"method", info.FullMethod,
			"error", err.Error(),
		}
		if traceID != "" {
			logAttrs = append(logAttrs, "trace_id", traceID)
		}

		if appErr, ok := errors.AsType[*apperror.Error](err); ok {
			slog.ErrorContext(ctx, "request failed", logAttrs...)

			grpcCode, exists := codeMap[appErr.Code()]
			if !exists {
				grpcCode = codes.Internal
			}

			return nil, status.Error(grpcCode, messageWithTraceID(appErr.Message(), traceID))
		}

		slog.ErrorContext(ctx, "unexpected error", logAttrs...)

		return nil, status.Error(codes.Internal, messageWithTraceID("internal server error", traceID))
	}
}

func extractTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}

	return ""
}

func messageWithTraceID(message, traceID string) string {
	if traceID == "" {
		return message
	}

	return message + " (trace_id=" + traceID + ")"
}
