package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cloudpulse/cloudpulse-go/backend/internal/middleware"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/models"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/repository"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/services"
	"github.com/cloudpulse/cloudpulse-go/backend/pkg/response"
)

func isValidEnvironment(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "production", "staging", "development", "testing", "other":
		return true
	default:
		return false
	}
}

func (a *API) handleListServers(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}

	filter := repository.ServerFilter{
		Search:      r.URL.Query().Get("search"),
		Environment: r.URL.Query().Get("environment"),
		ServerGroup: r.URL.Query().Get("server_group"),
		Page:        page,
		PageSize:    pageSize,
		SortBy:      r.URL.Query().Get("sort_by"),
		SortDir:     r.URL.Query().Get("sort_dir"),
	}
	healthFilter := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("health")))

	servers, total, err := a.store.ListServers(r.Context(), claims.OrganizationID, filter)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not list servers")
		return
	}

	servers, promOnline := a.mon.EnrichServers(r.Context(), claims.OrganizationID, servers)

	if healthFilter != "" && healthFilter != "ALL" {
		filtered := make([]models.Server, 0, len(servers))
		for _, srv := range servers {
			if strings.EqualFold(srv.Status, healthFilter) || strings.EqualFold(srv.Availability, healthFilter) {
				filtered = append(filtered, srv)
			}
		}
		servers = filtered
		total = len(filtered)
	}

	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}

	response.JSON(w, r, http.StatusOK, map[string]any{
		"items":                servers,
		"total":                total,
		"page":                 page,
		"page_size":            pageSize,
		"total_pages":          totalPages,
		"prometheus_available": promOnline,
	}, "Servers loaded")
}

func (a *API) handleCreateServer(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	var req struct {
		Name               string             `json:"name"`
		Hostname           string             `json:"hostname"`
		Environment        string             `json:"environment"`
		OperatingSystem    string             `json:"operating_system"`
		Architecture       string             `json:"architecture"`
		Description        string             `json:"description"`
		ServerGroup        string             `json:"server_group"`
		ScrapeAddress      string             `json:"scrape_address"`
		MonitoringInterval int                `json:"monitoring_interval"`
		Tags               []models.ServerTag `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Hostname = strings.TrimSpace(req.Hostname)
	req.Environment = strings.ToLower(strings.TrimSpace(req.Environment))
	req.ScrapeAddress = strings.TrimSpace(req.ScrapeAddress)
	req.ServerGroup = strings.TrimSpace(req.ServerGroup)
	req.OperatingSystem = strings.ToLower(strings.TrimSpace(req.OperatingSystem))
	req.Architecture = strings.TrimSpace(req.Architecture)

	if req.Name == "" || req.Hostname == "" || req.ScrapeAddress == "" {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Name, hostname, and Prometheus scrape address are required")
		return
	}
	if !isValidEnvironment(req.Environment) {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Environment must be production, staging, development, testing, or other")
		return
	}
	if req.OperatingSystem == "" {
		req.OperatingSystem = "linux"
	}
	if req.Architecture == "" {
		req.Architecture = "amd64"
	}
	if req.ServerGroup == "" {
		req.ServerGroup = "default"
	}
	if req.MonitoringInterval < 5 {
		req.MonitoringInterval = 15
	}

	srv, err := a.store.CreateServer(r.Context(), models.Server{
		OrganizationID:     claims.OrganizationID,
		Name:               req.Name,
		Hostname:           req.Hostname,
		Environment:        req.Environment,
		OperatingSystem:    req.OperatingSystem,
		Architecture:       req.Architecture,
		Description:        strings.TrimSpace(req.Description),
		ServerGroup:        req.ServerGroup,
		ScrapeAddress:      req.ScrapeAddress,
		MonitoringInterval: req.MonitoringInterval,
		MonitoringEnabled:  true,
		Tags:               req.Tags,
	})
	if errors.Is(err, repository.ErrConflict) {
		response.Error(w, r, http.StatusConflict, "SERVER_EXISTS", "A server with this scrape address is already registered")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not register server")
		return
	}

	// Newly registered server MUST initially be PENDING, never UP (PRD 8.2 & 37.3)
	srv.Status = models.StatusPending
	srv.Availability = models.StatusUnknown
	srv.DataSource = "live_prometheus"

	if err := a.mon.SyncDiscovery(r.Context()); err != nil {
		a.logger.Warn("file_sd sync failed after server creation", "error", err)
	}

	now := time.Now().UTC()
	_, _ = a.store.CreateNotification(r.Context(), models.NotificationRecord{
		OrganizationID: claims.OrganizationID,
		Title:          "New server registered: " + srv.Name,
		Message:        "Server " + srv.Name + " (" + srv.ScrapeAddress + ") was registered and added to Prometheus file_sd discovery.",
		Category:       "infrastructure",
		Severity:       "info",
		Status:         "delivered",
		DeliveredAt:    &now,
	})

	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "server.registered", "server", srv.ID.String(), middleware.ClientIP(r), map[string]any{
		"name":           srv.Name,
		"scrape_address": srv.ScrapeAddress,
		"environment":    srv.Environment,
	})

	response.JSON(w, r, http.StatusCreated, srv, "Server registered; awaiting first Prometheus scrape")
}

func (a *API) handleGetServer(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid server ID")
		return
	}
	srv, err := a.store.GetServer(r.Context(), claims.OrganizationID, id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Server not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load server")
		return
	}
	enriched, _ := a.mon.EnrichServers(r.Context(), claims.OrganizationID, []models.Server{srv})
	response.JSON(w, r, http.StatusOK, enriched[0], "Server loaded")
}

func (a *API) handleUpdateServer(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid server ID")
		return
	}
	existing, err := a.store.GetServer(r.Context(), claims.OrganizationID, id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Server not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load server")
		return
	}

	var req struct {
		Name               *string            `json:"name"`
		Hostname           *string            `json:"hostname"`
		Environment        *string            `json:"environment"`
		OperatingSystem    *string            `json:"operating_system"`
		Architecture       *string            `json:"architecture"`
		Description        *string            `json:"description"`
		ServerGroup        *string            `json:"server_group"`
		ScrapeAddress      *string            `json:"scrape_address"`
		MonitoringInterval *int               `json:"monitoring_interval"`
		Tags               []models.ServerTag `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		existing.Name = strings.TrimSpace(*req.Name)
	}
	if req.Hostname != nil && strings.TrimSpace(*req.Hostname) != "" {
		existing.Hostname = strings.TrimSpace(*req.Hostname)
	}
	if req.Environment != nil {
		if !isValidEnvironment(*req.Environment) {
			response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid environment")
			return
		}
		existing.Environment = strings.ToLower(strings.TrimSpace(*req.Environment))
	}
	if req.OperatingSystem != nil && strings.TrimSpace(*req.OperatingSystem) != "" {
		existing.OperatingSystem = strings.ToLower(strings.TrimSpace(*req.OperatingSystem))
	}
	if req.Architecture != nil && strings.TrimSpace(*req.Architecture) != "" {
		existing.Architecture = strings.TrimSpace(*req.Architecture)
	}
	if req.Description != nil {
		existing.Description = strings.TrimSpace(*req.Description)
	}
	if req.ServerGroup != nil && strings.TrimSpace(*req.ServerGroup) != "" {
		existing.ServerGroup = strings.TrimSpace(*req.ServerGroup)
	}
	if req.ScrapeAddress != nil && strings.TrimSpace(*req.ScrapeAddress) != "" {
		existing.ScrapeAddress = strings.TrimSpace(*req.ScrapeAddress)
	}
	if req.MonitoringInterval != nil && *req.MonitoringInterval >= 5 {
		existing.MonitoringInterval = *req.MonitoringInterval
	}
	if req.Tags != nil {
		existing.Tags = req.Tags
	}

	updated, err := a.store.UpdateServer(r.Context(), existing)
	if errors.Is(err, repository.ErrConflict) {
		response.Error(w, r, http.StatusConflict, "SERVER_EXISTS", "Another server already uses that scrape address")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not update server")
		return
	}

	_ = a.mon.SyncDiscovery(r.Context())
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "server.updated", "server", updated.ID.String(), middleware.ClientIP(r), map[string]any{
		"name":           updated.Name,
		"scrape_address": updated.ScrapeAddress,
	})
	enriched, _ := a.mon.EnrichServers(r.Context(), claims.OrganizationID, []models.Server{updated})
	response.JSON(w, r, http.StatusOK, enriched[0], "Server updated")
}

func (a *API) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid server ID")
		return
	}
	srv, err := a.store.GetServer(r.Context(), claims.OrganizationID, id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Server not found")
		return
	}
	if err := a.store.DeleteServer(r.Context(), claims.OrganizationID, id); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not delete server")
		return
	}
	_ = a.mon.SyncDiscovery(r.Context())
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "server.deleted", "server", id.String(), middleware.ClientIP(r), map[string]any{
		"name": srv.Name,
		"note": "Removed from Prometheus file_sd discovery; historical TSDB samples follow Prometheus retention policy.",
	})
	response.JSON(w, r, http.StatusOK, map[string]any{
		"deleted":                   true,
		"historical_metrics_notice": "Server removed from active discovery. Historical metrics in Prometheus remain subject to configured retention policy.",
	}, "Server removed from discovery")
}

func (a *API) handleEnableServer(w http.ResponseWriter, r *http.Request) {
	a.toggleServerMonitoring(w, r, true)
}

func (a *API) handleDisableServer(w http.ResponseWriter, r *http.Request) {
	a.toggleServerMonitoring(w, r, false)
}

func (a *API) toggleServerMonitoring(w http.ResponseWriter, r *http.Request, enabled bool) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid server ID")
		return
	}
	srv, err := a.store.SetServerMonitoring(r.Context(), claims.OrganizationID, id, enabled)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Server not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not update monitoring state")
		return
	}
	_ = a.mon.SyncDiscovery(r.Context())
	action := "server.monitoring_disabled"
	if enabled {
		action = "server.monitoring_enabled"
	}
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, action, "server", id.String(), middleware.ClientIP(r), map[string]any{"name": srv.Name})
	enriched, _ := a.mon.EnrichServers(r.Context(), claims.OrganizationID, []models.Server{srv})
	response.JSON(w, r, http.StatusOK, enriched[0], "Server monitoring updated")
}

func (a *API) handleGetServerStatus(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid server ID")
		return
	}
	srv, err := a.store.GetServer(r.Context(), claims.OrganizationID, id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Server not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load server")
		return
	}
	enriched, promOnline := a.mon.EnrichServers(r.Context(), claims.OrganizationID, []models.Server{srv})
	s := enriched[0]
	collected := []string{}
	if s.CPUPercent != nil {
		collected = append(collected, "CPU")
	}
	if s.MemoryPercent != nil {
		collected = append(collected, "Memory")
	}
	if s.DiskPercent != nil {
		collected = append(collected, "Disk")
	}

	response.JSON(w, r, http.StatusOK, map[string]any{
		"server_id":              s.ID,
		"name":                   s.Name,
		"hostname":               s.Hostname,
		"scrape_address":         s.ScrapeAddress,
		"status":                 s.Status,
		"availability":           s.Availability,
		"connected":              s.Availability == models.StatusUp && !s.IsStale,
		"is_stale":               s.IsStale,
		"agent_version":          s.AgentVersion,
		"last_successful_scrape": s.LastSampleAt,
		"collected_metrics":      collected,
		"prometheus_available":   promOnline,
	}, "Server connection status loaded")
}

func (a *API) handleGetServerMetrics(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid server ID")
		return
	}
	data, err := a.mon.GetServerMetrics(r.Context(), claims.OrganizationID, id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Server not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not query server metrics")
		return
	}
	response.JSON(w, r, http.StatusOK, data, "Server metrics loaded")
}

func (a *API) handleGetServerHistory(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid server ID")
		return
	}
	rangeKey := r.URL.Query().Get("range")
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	data, err := a.mon.GetServerHistory(r.Context(), claims.OrganizationID, id, rangeKey, startStr, endStr)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Server not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not query server history")
		return
	}
	response.JSON(w, r, http.StatusOK, data, "Server history loaded")
}

func (a *API) handleMonitoringOverview(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	rangeKey := r.URL.Query().Get("range")
	data, err := a.mon.GetOverview(r.Context(), claims.OrganizationID, rangeKey)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not compute monitoring overview")
		return
	}
	response.JSON(w, r, http.StatusOK, data, "Monitoring overview loaded")
}

func (a *API) handleMonitoringHealth(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	data, err := a.mon.GetInfrastructureHealth(r.Context(), claims.OrganizationID)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load infrastructure health")
		return
	}
	response.JSON(w, r, http.StatusOK, data, "Infrastructure health loaded")
}

func (a *API) handleMonitoringMetrics(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	q := r.URL.Query()
	catalog := services.SupportedMetricCatalog()
	history, _ := a.store.ListQueryHistory(r.Context(), claims.OrganizationID, claims.UserID, 15)

	if q.Get("execute") != "true" && q.Get("query") == "" && q.Get("metric") == "" {
		response.JSON(w, r, http.StatusOK, map[string]any{
			"catalog": catalog,
			"history": history,
		}, "Metric catalog loaded")
		return
	}

	res, err := a.mon.ExecuteMetricsQuery(
		r.Context(),
		claims.OrganizationID,
		claims.UserID,
		q.Get("query"),
		q.Get("metric"),
		q.Get("server_id"),
		q.Get("environment"),
		q.Get("aggregation"),
		q.Get("range"),
		q.Get("start"),
		q.Get("end"),
	)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_PROMQL", err.Error())
		return
	}
	history, _ = a.store.ListQueryHistory(r.Context(), claims.OrganizationID, claims.UserID, 15)
	response.JSON(w, r, http.StatusOK, map[string]any{
		"catalog": catalog,
		"result":  res,
		"history": history,
	}, "Metrics query executed")
}

func (a *API) handleMonitoringTimeseries(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	q := r.URL.Query()
	data, err := a.mon.GetDashboardTimeseries(
		r.Context(),
		claims.OrganizationID,
		q.Get("server_id"),
		q.Get("environment"),
		q.Get("range"),
		q.Get("start"),
		q.Get("end"),
	)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load time-series data")
		return
	}
	response.JSON(w, r, http.StatusOK, data, "Time-series loaded")
}

func (a *API) handleMonitoringTargets(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	targets, err := a.prom.Targets(r.Context())
	if err != nil {
		response.JSON(w, r, http.StatusOK, map[string]any{
			"prometheus_available": false,
			"targets":              []any{},
		}, "Prometheus currently unavailable")
		return
	}
	orgStr := claims.OrganizationID.String()
	filtered := make([]any, 0)
	for _, t := range targets {
		if t.Labels["cloudpulse_org_id"] == orgStr || t.ScrapePool == "prometheus" {
			filtered = append(filtered, t)
		}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{
		"prometheus_available": true,
		"targets":              filtered,
	}, "Prometheus targets loaded")
}

// Service Check Handlers

func (a *API) handleListServices(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	var srvID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("server_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid server_id")
			return
		}
		srvID = &id
	}
	svcs, err := a.mon.ListServicesEnriched(r.Context(), claims.OrganizationID, srvID)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not list services")
		return
	}
	response.JSON(w, r, http.StatusOK, svcs, "Services loaded")
}

func (a *API) handleCreateService(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	var req struct {
		ServerID           string `json:"server_id"`
		Name               string `json:"name"`
		CheckType          string `json:"check_type"`
		Target             string `json:"target"`
		IntervalSeconds    int    `json:"interval_seconds"`
		TimeoutSeconds     int    `json:"timeout_seconds"`
		ExpectedStatusCode int    `json:"expected_status_code"`
		Enabled            *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}
	serverID, err := uuid.Parse(strings.TrimSpace(req.ServerID))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Valid server_id is required")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.CheckType = strings.ToLower(strings.TrimSpace(req.CheckType))
	req.Target = strings.TrimSpace(req.Target)
	if req.Name == "" || req.Target == "" {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Service name and target are required")
		return
	}
	if req.CheckType != "http" && req.CheckType != "tcp" {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "check_type must be http or tcp")
		return
	}
	if req.CheckType == "http" {
		u, err := url.Parse(req.Target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "HTTP target must be a valid http(s) URL")
			return
		}
		if u.User != nil {
			response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Service targets must not embed passwords or credentials")
			return
		}
	}
	if req.IntervalSeconds < 5 {
		req.IntervalSeconds = 30
	}
	if req.TimeoutSeconds < 1 {
		req.TimeoutSeconds = 5
	}
	if req.ExpectedStatusCode == 0 && req.CheckType == "http" {
		req.ExpectedStatusCode = 200
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	sc, err := a.store.CreateServiceCheck(r.Context(), claims.OrganizationID, models.ServiceCheck{
		ServerID:           serverID,
		Name:               req.Name,
		CheckType:          req.CheckType,
		Target:             req.Target,
		IntervalSeconds:    req.IntervalSeconds,
		TimeoutSeconds:     req.TimeoutSeconds,
		ExpectedStatusCode: req.ExpectedStatusCode,
		Enabled:            enabled,
	})
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Target server not found")
		return
	}
	if errors.Is(err, repository.ErrConflict) {
		response.Error(w, r, http.StatusConflict, "SERVICE_EXISTS", "A service check with that name already exists on this server")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not create service check")
		return
	}
	sc.Status = models.StatusUnknown
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "service.created", "service_check", sc.ID.String(), middleware.ClientIP(r), map[string]any{
		"name":       sc.Name,
		"check_type": sc.CheckType,
		"server_id":  sc.ServerID.String(),
	})
	response.JSON(w, r, http.StatusCreated, sc, "Service check created")
}

func (a *API) handleGetService(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid service ID")
		return
	}
	sc, err := a.store.GetServiceCheck(r.Context(), claims.OrganizationID, id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Service check not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load service check")
		return
	}
	response.JSON(w, r, http.StatusOK, sc, "Service check loaded")
}

func (a *API) handleUpdateService(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid service ID")
		return
	}
	existing, err := a.store.GetServiceCheck(r.Context(), claims.OrganizationID, id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Service check not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load service check")
		return
	}

	var req struct {
		Name               *string `json:"name"`
		CheckType          *string `json:"check_type"`
		Target             *string `json:"target"`
		IntervalSeconds    *int    `json:"interval_seconds"`
		TimeoutSeconds     *int    `json:"timeout_seconds"`
		ExpectedStatusCode *int    `json:"expected_status_code"`
		Enabled            *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		existing.Name = strings.TrimSpace(*req.Name)
	}
	if req.CheckType != nil && (*req.CheckType == "http" || *req.CheckType == "tcp") {
		existing.CheckType = *req.CheckType
	}
	if req.Target != nil && strings.TrimSpace(*req.Target) != "" {
		existing.Target = strings.TrimSpace(*req.Target)
	}
	if req.IntervalSeconds != nil && *req.IntervalSeconds >= 5 {
		existing.IntervalSeconds = *req.IntervalSeconds
	}
	if req.TimeoutSeconds != nil && *req.TimeoutSeconds >= 1 {
		existing.TimeoutSeconds = *req.TimeoutSeconds
	}
	if req.ExpectedStatusCode != nil && *req.ExpectedStatusCode >= 0 && *req.ExpectedStatusCode <= 599 {
		existing.ExpectedStatusCode = *req.ExpectedStatusCode
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	updated, err := a.store.UpdateServiceCheck(r.Context(), claims.OrganizationID, existing)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not update service check")
		return
	}
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "service.updated", "service_check", updated.ID.String(), middleware.ClientIP(r), map[string]any{
		"name":    updated.Name,
		"enabled": updated.Enabled,
	})
	response.JSON(w, r, http.StatusOK, updated, "Service check updated")
}

func (a *API) handleDeleteService(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid service ID")
		return
	}
	if err := a.store.DeleteServiceCheck(r.Context(), claims.OrganizationID, id); errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Service check not found")
		return
	} else if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not delete service check")
		return
	}
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "service.deleted", "service_check", id.String(), middleware.ClientIP(r), nil)
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true}, "Service check removed")
}

func (a *API) handleGetServiceHistory(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid service ID")
		return
	}
	data, err := a.mon.GetServiceHistory(r.Context(), claims.OrganizationID, id, r.URL.Query().Get("range"))
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Service check not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load service history")
		return
	}
	response.JSON(w, r, http.StatusOK, data, "Service availability history loaded")
}
