package flogger

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

func GrpcLogger(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	base := LoggerFromContext(ctx, nil)
	ctx, span := StartRequestSpan(ctx, base, info.FullMethod, logrus.Fields{
		"component": "grpc",
		"rpc_type":  "unary",
		"method":    info.FullMethod,
	})
	defer span.End()

	logger := span.Logger()
	start := time.Now()
	resp, err := handler(ctx, req)
	duration := time.Since(start)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"method":      info.FullMethod,
			"duration_ms": duration.Milliseconds(),
		}).WithError(ErrorWithContext(err, map[string]any{
			"method":      info.FullMethod,
			"duration_ms": duration.Milliseconds(),
		})).Error("grpc unary request failed")
		return resp, err
	}

	logger.WithFields(logrus.Fields{
		"method":      info.FullMethod,
		"duration_ms": duration.Milliseconds(),
	}).Info("grpc unary request completed")
	return resp, err
}

func GrpcStreamLogger(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	base := LoggerFromContext(ss.Context(), nil)
	ctx, logger := WithLogFields(ss.Context(), base, logrus.Fields{
		"component": "grpc",
		"rpc_type":  "stream",
		"method":    info.FullMethod,
	})

	start := time.Now()
	err := handler(srv, &wrappedServerStream{
		ServerStream: ss,
		ctx:          ctx,
	})
	duration := time.Since(start)
	logger = logger.WithFields(logrus.Fields{
		"method":      info.FullMethod,
		"duration_ms": duration.Milliseconds(),
	})
	if err != nil {
		logger.WithError(ErrorWithContext(err, map[string]any{
			"method":      info.FullMethod,
			"duration_ms": duration.Milliseconds(),
		})).Error("grpc stream request failed")
		return err
	}

	logger.Info("grpc stream request completed")
	return nil
}
