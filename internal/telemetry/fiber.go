package telemetry

import (
	"context"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/trace"
)

// AccessLog logs one line per request with trace_id from the active span (after otelfiber).
func AccessLog() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		tid := traceIDFromCtx(c.UserContext())
		log.Printf(
			"access method=%s path=%s status=%d latency_ms=%d trace_id=%s",
			c.Method(), c.Path(), c.Response().StatusCode(), time.Since(start).Milliseconds(), tid,
		)
		return err
	}
}

func traceIDFromCtx(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.HasTraceID() {
		return ""
	}
	return sc.TraceID().String()
}

// MountHealth registers /healthz (liveness) and /readyz/telemetry (OTLP reachability when OTEL is on).
func MountHealth(app *fiber.App, serviceName string) {
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})
	app.Get("/readyz/telemetry", func(c *fiber.Ctx) error {
		code, payload := telemetryReadinessJSON(serviceName)
		return c.Status(code).JSON(payload)
	})
}

func telemetryReadinessJSON(serviceName string) (int, map[string]any) {
	enabled := envBool("OTEL_ENABLED", true)
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		endpoint = "localhost:4317"
	}
	appEnv := defaultString(strings.TrimSpace(os.Getenv("APP_ENV")), "dev")

	out := map[string]any{
		"service":       serviceName,
		"app_env":       appEnv,
		"otel_enabled":  enabled,
		"otlp_endpoint": endpoint,
	}
	if !enabled {
		out["exporter_reachable"] = nil
		out["note"] = "OTEL disabled"
		return fiber.StatusOK, out
	}
	hostPort := normalizeOTLPEndpoint(endpoint)
	ok, errStr := checkEndpointTCP(hostPort, 2*time.Second)
	out["exporter_reachable"] = ok
	if errStr != "" {
		out["exporter_check_error"] = errStr
	}
	if !ok {
		return fiber.StatusServiceUnavailable, out
	}
	return fiber.StatusOK, out
}

func normalizeOTLPEndpoint(endpoint string) string {
	e := strings.TrimSpace(endpoint)
	e = strings.TrimPrefix(e, "http://")
	e = strings.TrimPrefix(e, "https://")
	return e
}

func checkEndpointTCP(hostPort string, d time.Duration) (ok bool, errMsg string) {
	if hostPort == "" {
		return false, "empty endpoint"
	}
	conn, err := net.DialTimeout("tcp", hostPort, d)
	if err != nil {
		return false, err.Error()
	}
	_ = conn.Close()
	return true, ""
}
