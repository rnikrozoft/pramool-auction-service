package telemetry

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// AccessLogWithZap emits one structured access log line per request (use after otelfiber).
func AccessLogWithZap(logger *zap.Logger) fiber.Handler {
	if logger == nil {
		return AccessLog()
	}
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		code := c.Response().StatusCode()
		tid := traceIDFromCtx(c.UserContext())
		fields := []zap.Field{
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.String("client_ip", c.IP()),
			zap.Int("status", code),
			zap.Int("response_bytes", len(c.Response().Body())),
			zap.Int64("latency_ms", time.Since(start).Milliseconds()),
			zap.String("trace_id", tid),
		}
		if ua := c.Get(fiber.HeaderUserAgent); ua != "" {
			fields = append(fields, zap.String("user_agent", ua))
		}
		if err != nil {
			fields = append(fields, zap.Error(err))
			logger.Warn("access", fields...)
			return err
		}
		switch {
		case code >= 500:
			logger.Error("access", fields...)
		case code >= 400:
			logger.Warn("access", fields...)
		default:
			logger.Info("access", fields...)
		}
		return err
	}
}
