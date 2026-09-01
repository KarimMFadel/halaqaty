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

	apirouter "github.com/KarimMFadel/halaqaty/backend/internal/api"
	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/config"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/profile"
	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

const (
	queueOutboxPollInterval = 100 * time.Millisecond
	queueOutboxBatchSize    = 100
)

type slogOutboxParkedAlerter struct {
	logger *slog.Logger
}

func (a slogOutboxParkedAlerter) AlertOutboxParked(_ context.Context, event queue.OutboxEvent) {
	if a.logger == nil {
		return
	}
	a.logger.Error(
		"queue outbox event parked",
		"event_id", event.EventID,
		"event_type", event.EventType,
		"attempt_count", event.AttemptCount,
	)
}

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
	queueRepo := queue.NewQueueRepository(pool)
	queueRounds := queue.NewRoundService(queueRepo)
	queueOptOuts := queue.NewOptOutService(queueRepo)
	queueHandler := queue.NewHandler(queueRepo, queueRounds, queue.NewTurnService(queueRepo), queue.NewPolicyService(queueRepo), queueOptOuts)

	var sessionHandler *sessions.Handler
	var liveSessionService *sessions.Service
	var sessionReconciler *sessions.Reconciler
	ticketService := realtime.NewTicketService(rbacRepo)
	realtimeHandler := realtime.NewHandler(ticketService)
	var sessionTopicAuthorizer realtime.SessionTopicAuthorizer
	mediaCfg, err := config.LoadLiveKitConfig()
	if err != nil {
		logger.Error("failed to load LiveKit config", "error", err)
		os.Exit(1)
	}
	if mediaCfg != (config.LiveKitConfig{}) {
		roomKey, err := config.LoadSessionRoomKey()
		if err != nil {
			logger.Error("failed to load session room key", "error", err)
			os.Exit(1)
		}
		policy, err := config.LoadAudioPolicy()
		if err != nil {
			logger.Error("failed to load LiveKit audio policy", "error", err)
			os.Exit(1)
		}
		rooms := lksdk.NewRoomServiceClient(mediaCfg.Endpoint, mediaCfg.APIKey, mediaCfg.APISecret)
		media := livekit.NewAdapter(mediaCfg, policy, rooms)
		liveVerifier := livekit.NewHandlerVerifier(mediaCfg.APIKey, mediaCfg.APISecret)
		liveSessionRepo := sessions.NewSessionRepository(pool)
		liveSessionService, err = sessions.NewServiceWithRoomKey(liveSessionRepo, media, rbacRepo, roomKey)
		if err != nil {
			logger.Error("failed to initialize live session service", "error", err)
			os.Exit(1)
		}
		sessionReconciler, err = sessions.NewReconciler(liveSessionRepo, media, roomKey)
		if err != nil {
			logger.Error("failed to initialize session reconciler", "error", err)
			os.Exit(1)
		}
		sessionTopicAuthorizer = liveSessionService
		sessionHandler = sessions.NewHandler(liveSessionService)
		sessionHandler.SetWebhookVerifier(liveVerifier)
	}

	authMetrics := new(metrics.AuthMetrics)
	authMW := middleware.NewAuthMiddleware(verifier, sessionService, sessionRepo)
	authMW.SetMetrics(authMetrics)
	roleMW := middleware.NewRoleMiddleware(sessionRepo)
	rateLimitMW := middleware.NewRateLimitMiddleware(cfg.RateLimitPerIPPerMin, cfg.RateLimitPerUserPerMin)

	realtimeHub := realtime.NewHub(ticketService, sessionTopicAuthorizer)
	queueProjector := queue.NewRealtimeOutboxProjector(queueRepo, realtimeHub)
	queueMetrics := new(metrics.QueueMetrics)
	queueConvergence := queue.NewConvergence(queueRepo, queueMetrics, logger)
	queueDispatcher := queue.NewOutboxDispatcher(
		queueRepo,
		queueProjector,
		queueMetrics,
		slogOutboxParkedAlerter{logger: logger},
		nil,
		nil,
	)
	realtimeHub.RegisterSessionEventProvider(queueProjector.QueueState)
	if liveSessionService != nil {
		liveSessionService.SetQueueObserver(sessions.NewBoundedQueueObserver(queue.NewSessionObserver(queueRounds, queueConvergence), 0))
		realtimeHub.SetSessionSnapshotProvider(liveSessionService.RealtimeSnapshot)
		realtimeHub.SetSessionCommandHandler(liveSessionService.HandleRealtimeCommand)
	}

	mwSet := apirouter.MiddlewareSet{
		Auth:            authMW,
		Role:            roleMW,
		RateLimit:       rateLimitMW,
		AuthHandler:     authHandler,
		ProfileHandler:  profileHandler,
		RBACHandler:     rbacHandler,
		SessionHandler:  sessionHandler,
		RealtimeHandler: realtimeHandler,
		RealtimeHub:     realtimeHub,
		QueueHandler:    queueHandler,
		Timeout:         cfg.RequestTimeout,
		Logger:          logger,
		Metrics:         authMetrics,
		QueueMetrics:    queueMetrics,
		MetricsToken:    cfg.MetricsToken,
	}

	// ── Router ────────────────────────────────────────────────────────────────
	router := apirouter.NewRouter(mwSet)

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
	reconcilerCtx, stopReconciler := context.WithCancel(context.Background())
	defer stopReconciler()

	// F-003 startup reconciliation: finalize any rounds left active/prepared
	// after a previous crash or missed session-end observer callback.
	if err := queueConvergence.Reconcile(reconcilerCtx); err != nil {
		logger.Error("queue startup convergence failed", "error", err)
	}
	if err := queueDispatcher.Replay(reconcilerCtx, queueOutboxBatchSize); err != nil {
		logger.Error("queue outbox replay failed", "error", err)
	}

	go func() {
		ticker := time.NewTicker(queueOutboxPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-reconcilerCtx.Done():
				return
			case <-ticker.C:
				if err := queueDispatcher.DispatchDue(reconcilerCtx, queueOutboxBatchSize); err != nil {
					logger.Error("queue outbox dispatch failed", "error", err)
				}
			}
		}
	}()
	if sessionReconciler != nil {
		go func() {
			if err := sessionReconciler.Run(reconcilerCtx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("session reconciliation stopped", "error", err)
			}
		}()
	}

	go func() {
		logger.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	logger.Info("shutting down...")
	stopReconciler()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
	logger.Info("server stopped")
}
