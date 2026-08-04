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

	firebaseAdmin "firebase.google.com/go/v4"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/config"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/profile"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.LoadAuthConfig()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// ── Database ──────────────────────────────────────────────────────────────
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Error("database ping failed", "error", err)
		os.Exit(1)
	}

	// ── Firebase ──────────────────────────────────────────────────────────────
	firebaseApp, err := firebaseAdmin.NewApp(context.Background(), &firebaseAdmin.Config{
		ProjectID: cfg.FirebaseProjectID,
	})
	if err != nil {
		logger.Error("failed to init Firebase app", "error", err)
		os.Exit(1)
	}

	firebaseAuthClient, err := firebaseApp.Auth(context.Background())
	if err != nil {
		logger.Error("failed to init Firebase auth client", "error", err)
		os.Exit(1)
	}

	// ── Dependencies ──────────────────────────────────────────────────────────
	sessionRepo := auth.NewSessionRepository(pool)
	sessionService := auth.NewSessionService(cfg.SessionInactivityTTL)
	verifier := auth.NewFirebaseVerifier(firebaseAuthClient)

	auditLogger := logging.NewAuditLogger(logger)

	authService := auth.NewService(sessionRepo, auditLogger, cfg.SessionAbsoluteTTL)
	authHandler := auth.NewHandler(authService)
	profileRepo := profile.NewRepository(pool)
	profileService := profile.NewService(profileRepo)
	profileHandler := profile.NewHandler(profileService)
	rbacRepo := rbac.NewRepository(pool)
	rbacService := rbac.NewService(rbacRepo, auditLogger)
	rbacHandler := rbac.NewHandler(rbacService)

	authMW := middleware.NewAuthMiddleware(verifier, sessionService, sessionRepo)
	roleMW := middleware.NewRoleMiddleware(sessionRepo)
	rateLimitMW := middleware.NewRateLimitMiddleware(cfg.RateLimitPerIPPerMin, cfg.RateLimitPerUserPerMin)

	mwSet := MiddlewareSet{
		Auth:           authMW,
		Role:           roleMW,
		RateLimit:      rateLimitMW,
		AuthHandler:    authHandler,
		ProfileHandler: profileHandler,
		RBACHandler:    rbacHandler,
		Timeout:        cfg.RequestTimeout,
		Logger:         logger,
	}

	// ── Router ────────────────────────────────────────────────────────────────
	router := NewRouter(mwSet)

	// ── Server ────────────────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		logger.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	logger.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
	logger.Info("server stopped")
}
