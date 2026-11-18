package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cloudpulse/cloudpulse-go/backend/internal/alertmanager"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/audit"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/auth"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/config"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/grafana"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/middleware"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/models"
	prom "github.com/cloudpulse/cloudpulse-go/backend/internal/prometheus"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/repository"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/services"
)

// API wires together HTTP handlers, services, and security middleware.
type API struct {
	cfg     config.Config
	store   repository.Store
	tokens  *auth.TokenManager
	mon     *services.MonitoringService
	prom    prom.Client
	am      alertmanager.Client
	grafana *grafana.Client
	audit   *audit.Recorder
	logger  *slog.Logger
}

func NewAPI(
	cfg config.Config,
	store repository.Store,
	tokens *auth.TokenManager,
	mon *services.MonitoringService,
	promClient prom.Client,
	amClient alertmanager.Client,
	gfClient *grafana.Client,
	aud *audit.Recorder,
	logger *slog.Logger,
) *API {
	if logger == nil {
		logger = slog.Default()
	}
	return &API{
		cfg:     cfg,
		store:   store,
		tokens:  tokens,
		mon:     mon,
		prom:    promClient,
		am:      amClient,
		grafana: gfClient,
		audit:   aud,
		logger:  logger,
	}
}

func (a *API) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer(a.logger))
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(a.cfg.CORSAllowedOrigins))
	r.Use(middleware.BodyLimit(2 << 20)) // 2 MiB max payload
	r.Use(middleware.Timeout(20 * time.Second))
	r.Use(middleware.StructuredLogger(a.logger))

	// Liveness & readiness probes
	r.Get("/healthz", a.handleHealthz)
	r.Get("/readyz", a.handleReadyz)

	authLimiter := middleware.NewRateLimiter(20, time.Minute)
	apiLimiter := middleware.NewRateLimiter(300, time.Minute)

	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Use(apiLimiter.Middleware)

		// Public version endpoint
		v1.Get("/system/version", a.handleSystemVersion)

		// Public authentication endpoints (rate-limited)
		v1.Route("/auth", func(ar chi.Router) {
			ar.With(authLimiter.Middleware).Post("/register", a.handleRegister)
			ar.With(authLimiter.Middleware).Post("/login", a.handleLogin)
			ar.With(authLimiter.Middleware).Post("/forgot-password", a.handleForgotPassword)
			ar.With(authLimiter.Middleware).Post("/reset-password", a.handleResetPassword)

			// Cookie-authenticated session renewal & logout require CSRF protection
			ar.With(middleware.VerifyCSRF).Post("/refresh", a.handleRefresh)
			ar.With(middleware.VerifyCSRF).Post("/logout", a.handleLogout)

			ar.With(middleware.Authenticate(a.tokens)).Get("/me", a.handleMe)
		})

		// Protected API routes
		v1.Group(func(pr chi.Router) {
			pr.Use(middleware.Authenticate(a.tokens))

			// Organization
			pr.Get("/organizations/current", a.handleGetCurrentOrg)
			pr.With(middleware.RequireRoles(models.RoleAdmin)).Patch("/organizations/current", a.handleUpdateCurrentOrg)

			// User Management (Admin only)
			pr.Route("/users", func(ur chi.Router) {
				ur.Use(middleware.RequireRoles(models.RoleAdmin))
				ur.Get("/", a.handleListUsers)
				ur.Post("/", a.handleCreateUser)
				ur.Patch("/{id}", a.handleUpdateUser)
				ur.Post("/{id}/disable", a.handleDisableUser)
				ur.Post("/{id}/reset-access", a.handleResetUserAccess)
			})

			// Audit Logs (append-only, Admin & Operator read)
			pr.With(middleware.RequireRoles(models.RoleAdmin, models.RoleOperator)).Get("/audit-logs", a.handleListAuditLogs)

			// Servers
			pr.Route("/servers", func(sr chi.Router) {
				sr.Get("/", a.handleListServers)
				sr.With(middleware.RequireRoles(models.RoleAdmin)).Post("/", a.handleCreateServer)
				sr.Get("/{id}", a.handleGetServer)
				sr.With(middleware.RequireRoles(models.RoleAdmin)).Patch("/{id}", a.handleUpdateServer)
				sr.With(middleware.RequireRoles(models.RoleAdmin)).Delete("/{id}", a.handleDeleteServer)
				sr.With(middleware.RequireRoles(models.RoleAdmin)).Post("/{id}/enable", a.handleEnableServer)
				sr.With(middleware.RequireRoles(models.RoleAdmin)).Post("/{id}/disable", a.handleDisableServer)
				sr.Get("/{id}/status", a.handleGetServerStatus)
				sr.Get("/{id}/metrics", a.handleGetServerMetrics)
				sr.Get("/{id}/history", a.handleGetServerHistory)
			})

			// Monitoring
			pr.Route("/monitoring", func(mr chi.Router) {
				mr.Get("/overview", a.handleMonitoringOverview)
				mr.Get("/health", a.handleMonitoringHealth)
				mr.Get("/metrics", a.handleMonitoringMetrics)
				mr.Get("/timeseries", a.handleMonitoringTimeseries)
				mr.Get("/targets", a.handleMonitoringTargets)
			})

			// Services (Admin & Operator can configure; Viewer read-only)
			pr.Route("/services", func(sr chi.Router) {
				sr.Get("/", a.handleListServices)
				sr.With(middleware.RequireRoles(models.RoleAdmin, models.RoleOperator)).Post("/", a.handleCreateService)
				sr.Get("/{id}", a.handleGetService)
				sr.With(middleware.RequireRoles(models.RoleAdmin, models.RoleOperator)).Patch("/{id}", a.handleUpdateService)
				sr.With(middleware.RequireRoles(models.RoleAdmin, models.RoleOperator)).Delete("/{id}", a.handleDeleteService)
				sr.Get("/{id}/history", a.handleGetServiceHistory)
			})

			// Alerts
			pr.Route("/alerts", func(ar chi.Router) {
				ar.Get("/", a.handleListAlerts)
				ar.Get("/history", a.handleAlertHistory)
				ar.Get("/{fingerprint}", a.handleGetAlertDetail)
				ar.With(middleware.RequireRoles(models.RoleAdmin, models.RoleOperator)).Post("/{fingerprint}/acknowledge", a.handleAcknowledgeAlert)
			})

			// Alert Rules (Admin manages; Operator & Viewer read)
			pr.Route("/alert-rules", func(rr chi.Router) {
				rr.Get("/", a.handleListAlertRules)
				rr.With(middleware.RequireRoles(models.RoleAdmin)).Post("/", a.handleCreateAlertRule)
				rr.Get("/{id}", a.handleGetAlertRule)
				rr.With(middleware.RequireRoles(models.RoleAdmin)).Patch("/{id}", a.handleUpdateAlertRule)
				rr.With(middleware.RequireRoles(models.RoleAdmin)).Delete("/{id}", a.handleDeleteAlertRule)
				rr.With(middleware.RequireRoles(models.RoleAdmin)).Post("/{id}/validate", a.handleValidateAlertRule)
			})

			// Notifications
			pr.Route("/notifications", func(nr chi.Router) {
				nr.Get("/", a.handleListNotifications)
				nr.Patch("/{id}/read", a.handleMarkNotificationRead)
				nr.Post("/read-all", a.handleMarkAllNotificationsRead)
			})

			// Notification Channels (Admin manages; others read-only summary)
			pr.Route("/notification-channels", func(nc chi.Router) {
				nc.Get("/", a.handleListNotificationChannels)
				nc.With(middleware.RequireRoles(models.RoleAdmin)).Post("/", a.handleCreateNotificationChannel)
				nc.With(middleware.RequireRoles(models.RoleAdmin)).Patch("/{id}", a.handleUpdateNotificationChannel)
				nc.With(middleware.RequireRoles(models.RoleAdmin)).Delete("/{id}", a.handleDeleteNotificationChannel)
				nc.With(middleware.RequireRoles(models.RoleAdmin)).Post("/{id}/test", a.handleTestNotificationChannel)
			})

			// Reports
			pr.Route("/reports", func(rr chi.Router) {
				rr.Get("/infrastructure", a.handleReportInfrastructure)
				rr.Get("/resources", a.handleReportResources)
				rr.Get("/services", a.handleReportServices)
				rr.Get("/alerts", a.handleReportAlerts)
				rr.Get("/export", a.handleReportExport)
			})

			// System Integrations
			pr.Get("/system/integrations", a.handleSystemIntegrations)
		})
	})

	return r
}
