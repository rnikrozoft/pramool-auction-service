package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/contrib/otelfiber"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/joho/godotenv"
	"github.com/rnikrozoft/pramool-auction-service/handler"
	"github.com/rnikrozoft/pramool-auction-service/internal/auctionlive"
	"github.com/rnikrozoft/pramool-auction-service/internal/config"
	"github.com/rnikrozoft/pramool-auction-service/internal/telemetry"
	"github.com/rnikrozoft/pramool-auction-service/middleware"
	"github.com/rnikrozoft/pramool-auction-service/repository"
	"github.com/rnikrozoft/pramool-auction-service/service"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"go.uber.org/zap"
)

func main() {
	_ = godotenv.Load()
	logger, err := telemetry.NewZapLogger()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	shutdownTelemetry, err := telemetry.Init("pramool-auction-service")
	if err != nil {
		logger.Fatal("otel init", zap.Error(err))
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTelemetry(ctx); err != nil {
			logger.Warn("otel shutdown", zap.Error(err))
		}
	}()

	jwtSecret := os.Getenv("JWT_SECRET")
	port := os.Getenv("PORT")
	if port == "" {
		port = "3103"
	}

	db, err := openDB()
	if err != nil {
		logger.Fatal("database", zap.Error(err))
	}
	defer db.Close()

	app := fiber.New(fiber.Config{
		AppName:   "pramool-auction-service",
		BodyLimit: 32 * 1024 * 1024,
	})
	app.Use(otelfiber.Middleware())
	app.Use(telemetry.AccessLogWithZap(logger))
	telemetry.MountHealth(app, "pramool-auction-service")
	corsOrigins := strings.TrimSpace(os.Getenv("CORS_ALLOW_ORIGINS"))
	if corsOrigins == "" {
		corsOrigins = "http://localhost:3000"
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins:     corsOrigins,
		AllowOriginsFunc: corsAllowDevLAN,
		AllowCredentials: true,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, Cookie",
	}))

	auctionRepo := repository.NewAuctionRepository(db)
	userCreditRepo := repository.NewUserCreditRepository(db)
	hub := service.NewAuctionHub()

	liveCache, err := auctionlive.NewRedisFromURL(auctionlive.RedisURLFromEnv())
	if err != nil {
		logger.Warn("redis live bidders disabled", zap.Error(err))
		liveCache = auctionlive.Noop()
	} else if liveCache.Enabled() {
		logger.Info("redis live bidders enabled")
	}
	defer func() { _ = auctionlive.Close(liveCache) }()

	platformFees := config.LoadPlatformFeesFromEnv()
	auctionSvc := service.NewAuctionService(
		auctionRepo,
		userCreditRepo,
		hub,
		strings.TrimSpace(os.Getenv("ESCROW_AUTO_CONFIRM_DAYS")),
		liveCache,
		platformFees,
	)
	auctionHandler := handler.NewAuctionHandler(auctionSvc)
	rt := handler.NewRealtimeHandler(hub, auctionSvc)
	m := middleware.Middleware{JWTSecret: jwtSecret}

	app.Static("/uploads", "./uploads")

	app.Get("/auctions", auctionHandler.ListAuctions)
	app.Get("/auctions/presence", rt.AuctionPresence)
	app.Get("/auctions/:id/bidders", auctionHandler.ListAuctionBidders)
	app.Get("/auctions/:id", auctionHandler.AuctionDetail)
	app.Get("/public/users/:id", auctionHandler.PublicUserProfile)
	app.Get("/my/active-bids", m.JWTMiddleware, auctionHandler.MyActiveBids)
	app.Get("/my/bid-history", m.JWTMiddleware, auctionHandler.MyBidHistory)
	app.Post("/auctions/:id/mark-shipped", m.JWTMiddleware, auctionHandler.MarkSellerShipped)
	app.Post("/auctions/:id/confirm-received", m.JWTMiddleware, auctionHandler.ConfirmBuyerReceived)
	app.Post("/auctions/:id/close-early", m.JWTMiddleware, auctionHandler.CloseEarly)

	app.Post("/seller/auctions", m.JWTMiddleware, auctionHandler.CreateSellerAuction)
	app.Get("/seller/auctions", m.JWTMiddleware, auctionHandler.ListSellerAuctions)
	app.Post("/seller/auctions/:id/reopen", m.JWTMiddleware, auctionHandler.ReopenSellerAuction)
	app.Delete("/seller/auctions/:id", m.JWTMiddleware, auctionHandler.DeleteSellerAuction)

	app.Get("/ws/auctions/:id", m.JWTMiddleware, func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	}, websocket.New(rt.AuctionWS))

	addr := ":" + port
	go func() {
		if err := app.Listen(addr); err != nil {
			logger.Error("listen stopped", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logger.Error("fiber shutdown", zap.Error(err))
	}
}

func openDB() (*bun.DB, error) {
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		host := os.Getenv("DATABASE_HOST")
		port := os.Getenv("DATABASE_PORT")
		user := os.Getenv("DATABASE_USERNAME")
		pass := os.Getenv("DATABASE_PASSWORD")
		name := os.Getenv("DATABASE_NAME")
		if host == "" || user == "" || name == "" {
			return nil, fmt.Errorf("set DATABASE_URL or DATABASE_HOST, DATABASE_USERNAME, DATABASE_NAME (and optional DATABASE_PORT, DATABASE_PASSWORD)")
		}
		if port == "" {
			port = "5432"
		}
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
	}
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	return bun.NewDB(sqldb, pgdialect.New()), nil
}
