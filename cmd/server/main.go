package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eisen/teamchat/internal/server/call"
	"github.com/eisen/teamchat/internal/server/chat"
	"github.com/eisen/teamchat/internal/server/httpapi"
	"github.com/eisen/teamchat/internal/server/presence"
	"github.com/eisen/teamchat/internal/server/store"
	"github.com/eisen/teamchat/internal/shared/config"
	"github.com/eisen/teamchat/internal/shared/logging"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.LoadServer()
	if err != nil {
		panic(err)
	}

	logger := logging.New(cfg.LogFormat, slog.LevelInfo)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbpool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect postgres", "error", err)
		os.Exit(1)
	}
	defer dbpool.Close()

	var redisClient *redis.Client
	if cfg.RedisAddr != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     cfg.RedisAddr,
			Username: cfg.RedisUser,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		})
		if err := redisClient.Ping(ctx).Err(); err != nil {
			logger.Warn("redis unavailable, continuing without redis", "error", err)
			redisClient = nil
		}
	}

	st := store.NewPostgres(dbpool)
	presenceSvc := presence.NewManager(logger, redisClient)
	callMgr := call.NewNoopManager(logger)
	hub := chat.NewHub(logger, st, presenceSvc, callMgr, cfg.DefaultChannel, cfg.HistoryLimit)
	server := httpapi.NewServer(logger, hub, cfg)

	go hub.Run(ctx)

	go func() {
		logger.Info("server starting", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("server shutting down")
	hub.Shutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown failed", "error", err)
	}
}
