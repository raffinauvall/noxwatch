package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/raffinauvall/noxwatch/apps/api/internal/alerts"
	"github.com/raffinauvall/noxwatch/apps/api/internal/config"
	"github.com/raffinauvall/noxwatch/apps/api/internal/database"
	"github.com/raffinauvall/noxwatch/apps/api/internal/enrollment"
	"github.com/raffinauvall/noxwatch/apps/api/internal/httpserver"
	"github.com/raffinauvall/noxwatch/apps/api/internal/notifications"
	"github.com/raffinauvall/noxwatch/apps/api/internal/observability"
)

func main() {
	var migrateMode string
	var healthcheck bool
	flag.StringVar(&migrateMode, "migrate", "", "run migrations: up or down")
	flag.BoolVar(&healthcheck, "healthcheck", false, "check local health endpoint")
	flag.Parse()

	if healthcheck {
		checkHealth()
		return
	}

	logger := observability.NewLogger(os.Stdout)
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if migrateMode != "" {
		if err := database.Migrate(ctx, db, cfg.MigrationsDir, migrateMode); err != nil {
			logger.Error("migration failed", "mode", migrateMode, "error", err)
			os.Exit(1)
		}
		logger.Info("migration complete", "mode", migrateMode)
		return
	}

	srv := httpserver.New(cfg, logger, db, func(ctx context.Context) error {
		if err := db.Ping(ctx); err != nil {
			return fmt.Errorf("postgres: %w", err)
		}
		if err := database.PingRedis(ctx, cfg.RedisAddr); err != nil {
			return fmt.Errorf("redis: %w", err)
		}
		return nil
	})
	monitorCtx, stopMonitor := context.WithCancel(context.Background())
	defer stopMonitor()
	alertService := alerts.NewService(db, notifications.NewService(db, cfg.AuthSecret, cfg.PublicWebURL, cfg.AppEnv == "development"))
	go monitorServerStatus(monitorCtx, enrollment.NewService(db), alertService, logger)

	go func() {
		logger.Info("api listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("api server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("api shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("api stopped")
}

func monitorServerStatus(ctx context.Context, service *enrollment.Service, alertService *alerts.Service, logger interface{ Error(string, ...any) }) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := service.MarkOffline(ctx); err != nil && ctx.Err() == nil {
				logger.Error("server status check failed", "error", err)
			}
			if err := alertService.EvaluateConnectivity(ctx); err != nil && ctx.Err() == nil {
				logger.Error("connectivity alert evaluation failed", "error", err)
			}
		}
	}
}

func checkHealth() {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:8080/health")
	if err != nil {
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}
