package telemetry

import (
	"os"
	"strings"

	"go.uber.org/zap"
)

// NewZapLogger uses JSON production config unless APP_ENV=dev (console development).
func NewZapLogger() (*zap.Logger, error) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "dev") {
		return zap.NewDevelopment()
	}
	return zap.NewProduction()
}
