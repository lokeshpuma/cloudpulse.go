package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cloudpulse/cloudpulse-go/backend/internal/middleware"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/models"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/notifications"
	prom "github.com/cloudpulse/cloudpulse-go/backend/internal/prometheus"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/reports"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/repository"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/services"
	"github.com/cloudpulse/cloudpulse-go/backend/pkg/response"
)

// Alerts & Alert Rules Handlers

func (a *API) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	q := r.URL.Query()
	alerts, err := a.mon.ListActiveAlerts(r.Context(), claims.OrganizationID, services.AlertFilter{
		Severity:     q.Get("severity"),
		Server:       q.Get("server"),
		Environment:  q.Get("environment"),
		State:        q.Get("state"),
		Acknowledged: q.Get("acknowledged"),
	})
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load alerts")
		return
	}
	response.JSON(w, r, http.StatusOK, alerts, "Alerts loaded")
}

func (a *API) handleAlertHistory(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	acks, err := a.store.ListAcknowledgementHistory(r.Context(), claims.OrganizationID, "")
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load alert history")
		return
	}
	active, _ := a.mon.ListActiveAlerts(r.Context(), claims.OrganizationID, services.AlertFilter{State: "all"})
	response.JSON(w, r, http.StatusOK, map[string]any{
		"alerts":           active,
		"acknowledgements": acks,
	}, "Alert history loaded")
}

func (a *API) handleGetAlertDetail(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	fp := strings.TrimSpace(chi.URLParam(r, "fingerprint"))
	if fp == "" {
		response.Error(w, r, http.StatusBadRequest, "INVALID_FINGERPRINT", "Alert fingerprint is required")
		return
	}
	detail, found, err := a.mon.GetAlertDetail(r.Context(), claims.OrganizationID, fp)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load alert detail")
		return
	}
	if !found {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Alert not found")
		return
	}
	response.JSON(w, r, http.StatusOK, detail, "Alert detail loaded")
}

func (a *API) handleAcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	fp := strings.TrimSpace(chi.URLParam(r, "fingerprint"))
	if fp == "" {
		response.Error(w, r, http.StatusBadRequest, "INVALID_FINGERPRINT", "Alert fingerprint is required")
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	ack, err := a.store.AcknowledgeAlert(r.Context(), claims.OrganizationID, fp, claims.UserID, strings.TrimSpace(req.Note))
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not acknowledge alert")
		return
	}
	ack.AcknowledgedName = claims.Name

	now := time.Now().UTC()
	_, _ = a.store.CreateNotification(r.Context(), models.NotificationRecord{
		OrganizationID:   claims.OrganizationID,
		AlertFingerprint: fp,
		Title:            "Alert acknowledged by " + claims.Name,
		Message:          "Alert " + fp + " was acknowledged in CloudPulse. Prometheus continues evaluating the underlying alert condition.",
		Category:         "alert",
		Severity:         "info",
		Status:           "delivered",
		DeliveredAt:      &now,
	})

	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "alert.acknowledged", "alert", fp, middleware.ClientIP(r), map[string]any{
		"note": ack.Note,
	})
	response.JSON(w, r, http.StatusCreated, ack, "Alert acknowledged (Prometheus alert remains active until condition resolves)")
}

func (a *API) handleListAlertRules(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	rules, err := a.store.ListAlertRules(r.Context(), claims.OrganizationID)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not list alert rules")
		return
	}
	response.JSON(w, r, http.StatusOK, rules, "Alert rules loaded")
}

func (a *API) handleGetAlertRule(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid alert rule ID")
		return
	}
	rule, err := a.store.GetAlertRule(r.Context(), claims.OrganizationID, id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Alert rule not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load alert rule")
		return
	}
	response.JSON(w, r, http.StatusOK, rule, "Alert rule loaded")
}

func (a *API) handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	var req struct {
		Name           string   `json:"name"`
		Description    string   `json:"description"`
		Expression     string   `json:"expression"`
		MetricTemplate string   `json:"metric_template"`
		Threshold      float64  `json:"threshold"`
		Severity       string   `json:"severity"`
		Duration       string   `json:"duration"`
		Environments   []string `json:"environments"`
		Enabled        *bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Severity = strings.ToLower(strings.TrimSpace(req.Severity))
	req.Duration = strings.TrimSpace(req.Duration)
	if req.Name == "" {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Rule name is required")
		return
	}
	if req.Severity != "critical" && req.Severity != "warning" && req.Severity != "info" {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Severity must be critical, warning, or info")
		return
	}
	if req.Duration == "" {
		req.Duration = "5m"
	}
	if _, err := time.ParseDuration(req.Duration); err != nil {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Duration must be a valid Go/Prometheus duration (e.g. 1m, 5m)")
		return
	}

	expr := strings.TrimSpace(req.Expression)
	if expr == "" && req.MetricTemplate != "" {
		expr = buildExpressionFromTemplate(req.MetricTemplate, req.Threshold)
	}
	if err := prom.ValidateControlledPromQL(expr); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_EXPRESSION", err.Error())
		return
	}

	envs := req.Environments
	if len(envs) == 0 {
		envs = []string{"production", "staging", "development", "testing", "other"}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	rule, err := a.store.CreateAlertRule(r.Context(), models.AlertRule{
		OrganizationID: claims.OrganizationID,
		Name:           req.Name,
		Description:    strings.TrimSpace(req.Description),
		Expression:     expr,
		MetricTemplate: firstNonEmptyStr(req.MetricTemplate, "custom"),
		Threshold:      req.Threshold,
		Severity:       req.Severity,
		Duration:       req.Duration,
		Environments:   envs,
		IsDefault:      false,
		Enabled:        enabled,
		LastEvalStatus: "validated",
		CreatedBy:      &claims.UserID,
	})
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not create alert rule")
		return
	}

	_ = a.mon.SyncOrgAlertRules(r.Context(), claims.OrganizationID)
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "alert_rule.created", "alert_rule", rule.ID.String(), middleware.ClientIP(r), map[string]any{
		"name":     rule.Name,
		"severity": rule.Severity,
	})
	response.JSON(w, r, http.StatusCreated, rule, "Alert rule created")
}

func (a *API) handleUpdateAlertRule(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid alert rule ID")
		return
	}
	existing, err := a.store.GetAlertRule(r.Context(), claims.OrganizationID, id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Alert rule not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load alert rule")
		return
	}

	var req struct {
		Name         *string  `json:"name"`
		Description  *string  `json:"description"`
		Expression   *string  `json:"expression"`
		Threshold    *float64 `json:"threshold"`
		Severity     *string  `json:"severity"`
		Duration     *string  `json:"duration"`
		Environments []string `json:"environments"`
		Enabled      *bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		existing.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		existing.Description = strings.TrimSpace(*req.Description)
	}
	if req.Threshold != nil {
		existing.Threshold = *req.Threshold
		if req.Expression == nil && existing.MetricTemplate != "" && existing.MetricTemplate != "custom" {
			existing.Expression = buildExpressionFromTemplate(existing.MetricTemplate, *req.Threshold)
		}
	}
	if req.Expression != nil && strings.TrimSpace(*req.Expression) != "" {
		if err := prom.ValidateControlledPromQL(*req.Expression); err != nil {
			response.Error(w, r, http.StatusBadRequest, "INVALID_EXPRESSION", err.Error())
			return
		}
		existing.Expression = strings.TrimSpace(*req.Expression)
	}
	if req.Severity != nil {
		sev := strings.ToLower(strings.TrimSpace(*req.Severity))
		if sev != "critical" && sev != "warning" && sev != "info" {
			response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid severity")
			return
		}
		existing.Severity = sev
	}
	if req.Duration != nil && strings.TrimSpace(*req.Duration) != "" {
		if _, err := time.ParseDuration(strings.TrimSpace(*req.Duration)); err != nil {
			response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid duration format")
			return
		}
		existing.Duration = strings.TrimSpace(*req.Duration)
	}
	if req.Environments != nil {
		existing.Environments = req.Environments
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	existing.LastEvalStatus = "updated"

	updated, err := a.store.UpdateAlertRule(r.Context(), existing)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not update alert rule")
		return
	}

	_ = a.mon.SyncOrgAlertRules(r.Context(), claims.OrganizationID)
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "alert_rule.updated", "alert_rule", updated.ID.String(), middleware.ClientIP(r), map[string]any{
		"name":      updated.Name,
		"enabled":   updated.Enabled,
		"threshold": updated.Threshold,
	})
	response.JSON(w, r, http.StatusOK, updated, "Alert rule updated")
}

func (a *API) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid alert rule ID")
		return
	}
	rule, err := a.store.GetAlertRule(r.Context(), claims.OrganizationID, id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Alert rule not found")
		return
	}
	if rule.IsDefault {
		response.Error(w, r, http.StatusBadRequest, "DEFAULT_RULE_PROTECTED", "Default rules can be disabled or edited, but not deleted")
		return
	}
	if err := a.store.DeleteAlertRule(r.Context(), claims.OrganizationID, id); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not delete alert rule")
		return
	}
	_ = a.mon.SyncOrgAlertRules(r.Context(), claims.OrganizationID)
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "alert_rule.deleted", "alert_rule", id.String(), middleware.ClientIP(r), map[string]any{"name": rule.Name})
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true}, "Alert rule deleted")
}

func (a *API) handleValidateAlertRule(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid alert rule ID")
		return
	}
	rule, err := a.store.GetAlertRule(r.Context(), claims.OrganizationID, id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Alert rule not found")
		return
	}
	if err := prom.ValidateControlledPromQL(rule.Expression); err != nil {
		response.JSON(w, r, http.StatusOK, map[string]any{"valid": false, "error": err.Error()}, "Expression validation failed")
		return
	}
	samples, promErr := a.prom.QueryInstant(r.Context(), rule.Expression, time.Now().UTC())
	if promErr != nil {
		response.JSON(w, r, http.StatusOK, map[string]any{
			"valid":                true,
			"prometheus_evaluated": false,
			"diagnostic":           promErr.Error(),
		}, "Syntax validated; Prometheus evaluation currently unavailable")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{
		"valid":                true,
		"prometheus_evaluated": true,
		"matching_series":      len(samples),
	}, "Alert rule expression validated against Prometheus")
}

func buildExpressionFromTemplate(tpl string, threshold float64) string {
	switch tpl {
	case "cpu_usage":
		return fmt.Sprintf("cloudpulse_cpu_usage_percent > %.2f", threshold)
	case "memory_usage":
		return fmt.Sprintf("cloudpulse_memory_usage_percent > %.2f", threshold)
	case "disk_usage":
		return fmt.Sprintf("cloudpulse_disk_usage_percent > %.2f", threshold)
	case "server_down":
		return `up{job="cloudpulse-agents"} == 0`
	case "service_down":
		return "cloudpulse_service_up == 0"
	case "network_errors":
		return fmt.Sprintf("rate(cloudpulse_network_receive_errors_total[5m]) + rate(cloudpulse_network_transmit_errors_total[5m]) > %.2f", threshold)
	default:
		return fmt.Sprintf("cloudpulse_cpu_usage_percent > %.2f", threshold)
	}
}

// Notification Center & Notification Channels Handlers

func (a *API) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}
	severity := strings.TrimSpace(r.URL.Query().Get("severity"))
	unreadOnly := strings.EqualFold(r.URL.Query().Get("unread_only"), "true")

	items, total, err := a.store.ListNotifications(r.Context(), claims.OrganizationID, severity, unreadOnly, page, pageSize)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load notifications")
		return
	}
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	response.JSON(w, r, http.StatusOK, response.Paginated{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, "Notifications loaded")
}

func (a *API) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid notification ID")
		return
	}
	if err := a.store.MarkNotificationRead(r.Context(), claims.OrganizationID, id); errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Notification not found")
		return
	} else if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not update notification")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]bool{"read": true}, "Notification marked as read")
}

func (a *API) handleMarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	if err := a.store.MarkAllNotificationsRead(r.Context(), claims.OrganizationID); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not mark notifications as read")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]bool{"read_all": true}, "All notifications marked as read")
}

func (a *API) handleListNotificationChannels(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	channels, err := a.store.ListNotificationChannels(r.Context(), claims.OrganizationID)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not list notification channels")
		return
	}
	response.JSON(w, r, http.StatusOK, channels, "Notification channels loaded")
}

func (a *API) handleCreateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	var req struct {
		Name                   string   `json:"name"`
		ChannelType            string   `json:"channel_type"`
		ConfigurationReference string   `json:"configuration_reference"`
		TargetSummary          string   `json:"target_summary"`
		SeverityFilters        []string `json:"severity_filters"`
		Enabled                *bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.ChannelType = strings.ToLower(strings.TrimSpace(req.ChannelType))
	req.ConfigurationReference = strings.TrimSpace(req.ConfigurationReference)
	if req.Name == "" {
		response.Error(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Channel name is required")
		return
	}
	if err := notifications.ValidateChannelConfig(req.ChannelType, req.ConfigurationReference, req.TargetSummary); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_CHANNEL_CONFIG", err.Error())
		return
	}
	sevs := req.SeverityFilters
	if len(sevs) == 0 {
		sevs = []string{"critical", "warning"}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	summary := strings.TrimSpace(req.TargetSummary)
	if summary == "" {
		summary = req.ConfigurationReference
	}

	ch, err := a.store.CreateNotificationChannel(r.Context(), models.NotificationChannel{
		OrganizationID:         claims.OrganizationID,
		Name:                   req.Name,
		ChannelType:            req.ChannelType,
		ConfigurationReference: req.ConfigurationReference,
		TargetSummary:          summary,
		SeverityFilters:        sevs,
		Enabled:                enabled,
	})
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not create notification channel")
		return
	}
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "notification_channel.created", "notification_channel", ch.ID.String(), middleware.ClientIP(r), map[string]any{
		"name":         ch.Name,
		"channel_type": ch.ChannelType,
	})
	response.JSON(w, r, http.StatusCreated, ch, "Notification channel configured")
}

func (a *API) handleUpdateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid channel ID")
		return
	}
	existing, err := a.store.GetNotificationChannel(r.Context(), claims.OrganizationID, id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Notification channel not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load notification channel")
		return
	}

	var req struct {
		Name                   *string  `json:"name"`
		ChannelType            *string  `json:"channel_type"`
		ConfigurationReference *string  `json:"configuration_reference"`
		TargetSummary          *string  `json:"target_summary"`
		SeverityFilters        []string `json:"severity_filters"`
		Enabled                *bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		existing.Name = strings.TrimSpace(*req.Name)
	}
	if req.ChannelType != nil && strings.TrimSpace(*req.ChannelType) != "" {
		existing.ChannelType = strings.ToLower(strings.TrimSpace(*req.ChannelType))
	}
	if req.ConfigurationReference != nil && strings.TrimSpace(*req.ConfigurationReference) != "" {
		existing.ConfigurationReference = strings.TrimSpace(*req.ConfigurationReference)
	}
	if err := notifications.ValidateChannelConfig(existing.ChannelType, existing.ConfigurationReference, existing.TargetSummary); err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_CHANNEL_CONFIG", err.Error())
		return
	}
	if req.TargetSummary != nil {
		existing.TargetSummary = strings.TrimSpace(*req.TargetSummary)
	}
	if req.SeverityFilters != nil {
		existing.SeverityFilters = req.SeverityFilters
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	updated, err := a.store.UpdateNotificationChannel(r.Context(), existing)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not update notification channel")
		return
	}
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "notification_channel.updated", "notification_channel", updated.ID.String(), middleware.ClientIP(r), map[string]any{
		"name":    updated.Name,
		"enabled": updated.Enabled,
	})
	response.JSON(w, r, http.StatusOK, updated, "Notification channel updated")
}

func (a *API) handleDeleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid channel ID")
		return
	}
	if err := a.store.DeleteNotificationChannel(r.Context(), claims.OrganizationID, id); errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Notification channel not found")
		return
	} else if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not delete notification channel")
		return
	}
	a.audit.Record(r.Context(), claims.OrganizationID, &claims.UserID, "notification_channel.deleted", "notification_channel", id.String(), middleware.ClientIP(r), nil)
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true}, "Notification channel deleted")
}

func (a *API) handleTestNotificationChannel(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid channel ID")
		return
	}
	ch, err := a.store.GetNotificationChannel(r.Context(), claims.OrganizationID, id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Notification channel not found")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load notification channel")
		return
	}

	now := time.Now().UTC()
	status := "delivered_via_alertmanager"
	errSummary := ""
	if a.am == nil {
		status = "failed"
		errSummary = "Alertmanager client is not configured"
	} else if err := a.am.SendTestAlert(r.Context(), ch.Name, "info"); err != nil {
		status = "failed"
		errSummary = err.Error()
	}

	_ = a.store.UpdateNotificationChannelTestStatus(r.Context(), claims.OrganizationID, id, status, now)
	var deliveredAt *time.Time
	if status != "failed" {
		deliveredAt = &now
	}
	_, _ = a.store.CreateNotification(r.Context(), models.NotificationRecord{
		OrganizationID: claims.OrganizationID,
		ChannelID:      &ch.ID,
		Title:          "Test notification: " + ch.Name,
		Message:        fmt.Sprintf("Dispatched test notification for %s (%s) through Alertmanager.", ch.Name, ch.ChannelType),
		Category:       "notification_test",
		Severity:       "info",
		Status:         status,
		DeliveredAt:    deliveredAt,
		ErrorSummary:   errSummary,
	})

	if status == "failed" {
		response.Error(w, r, http.StatusBadGateway, "ALERTMANAGER_ERROR", "Alertmanager test dispatch failed: "+errSummary)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{
		"status":    status,
		"tested_at": now,
	}, "Test notification routed through Alertmanager")
}

// Reports Handlers

func (a *API) handleReportInfrastructure(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	q := r.URL.Query()
	rep, err := a.mon.BuildInfrastructureReport(r.Context(), claims.OrganizationID, q.Get("environment"), q.Get("server_group"), q.Get("range"))
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not generate infrastructure report")
		return
	}
	response.JSON(w, r, http.StatusOK, rep, "Infrastructure report generated")
}

func (a *API) handleReportResources(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	q := r.URL.Query()
	rep, err := a.mon.BuildResourceReport(r.Context(), claims.OrganizationID, q.Get("environment"), q.Get("server_group"), q.Get("range"))
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not generate resource report")
		return
	}
	response.JSON(w, r, http.StatusOK, rep, "Resource utilization report generated")
}

func (a *API) handleReportServices(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	rep, err := a.mon.BuildServiceReport(r.Context(), claims.OrganizationID, r.URL.Query().Get("range"))
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not generate service report")
		return
	}
	response.JSON(w, r, http.StatusOK, rep, "Service availability report generated")
}

func (a *API) handleReportAlerts(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	q := r.URL.Query()
	rep, err := a.mon.BuildAlertReport(r.Context(), claims.OrganizationID, q.Get("environment"), q.Get("severity"), q.Get("range"))
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not generate alert history report")
		return
	}
	response.JSON(w, r, http.StatusOK, rep, "Alert history report generated")
}

func (a *API) handleReportExport(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFromContext(r.Context())
	q := r.URL.Query()
	repType := strings.ToLower(firstNonEmptyStr(q.Get("type"), "infrastructure"))
	format := strings.ToLower(firstNonEmptyStr(q.Get("format"), "csv"))
	env := q.Get("environment")
	group := q.Get("server_group")
	rangeKey := firstNonEmptyStr(q.Get("range"), "24h")
	severity := q.Get("severity")

	var title string
	var headers []string
	var rows [][]string
	var rawPayload any

	switch repType {
	case "resources":
		rep, err := a.mon.BuildResourceReport(r.Context(), claims.OrganizationID, env, group, rangeKey)
		if err != nil {
			response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Report export failed")
			return
		}
		rawPayload = rep
		title = "CloudPulse Resource Utilization Report"
		headers = []string{"Server", "Environment", "Group", "Avg CPU %", "Peak CPU %", "Avg Mem %", "Peak Mem %", "Disk %", "Net Rx (B/s)", "Net Tx (B/s)"}
		for _, row := range rep.Rows {
			rows = append(rows, []string{
				row.ServerName, row.Environment, row.ServerGroup,
				fmtFloatPtr(row.AvgCPU), fmtFloatPtr(row.PeakCPU),
				fmtFloatPtr(row.AvgMemory), fmtFloatPtr(row.PeakMemory),
				fmtFloatPtr(row.DiskUtilization), fmtFloatPtr(row.NetworkRxBps), fmtFloatPtr(row.NetworkTxBps),
			})
		}
	case "services":
		rep, err := a.mon.BuildServiceReport(r.Context(), claims.OrganizationID, rangeKey)
		if err != nil {
			response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Report export failed")
			return
		}
		rawPayload = rep
		title = "CloudPulse Service Availability Report"
		headers = []string{"Service", "Type", "Target", "Server", "Status", "Latency (ms)", "24h Availability %"}
		for _, s := range rep.Services {
			rows = append(rows, []string{
				s.Name, s.CheckType, s.Target, s.ServerName, s.Status,
				fmtFloatPtr(s.LatencyMs), fmtFloatPtr(s.Availability24h),
			})
		}
	case "alerts":
		rep, err := a.mon.BuildAlertReport(r.Context(), claims.OrganizationID, env, severity, rangeKey)
		if err != nil {
			response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Report export failed")
			return
		}
		rawPayload = rep
		title = "CloudPulse Alert History Report"
		headers = []string{"Title", "Severity", "State", "Server", "Environment", "Triggered At", "Acknowledged"}
		for _, al := range rep.Alerts {
			rows = append(rows, []string{
				al.Title, al.Severity, al.State, al.ServerName, al.Environment,
				al.TriggeredAt.Format(time.RFC3339), strconv.FormatBool(al.Acknowledged),
			})
		}
	default:
		rep, err := a.mon.BuildInfrastructureReport(r.Context(), claims.OrganizationID, env, group, rangeKey)
		if err != nil {
			response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Report export failed")
			return
		}
		rawPayload = rep
		title = "CloudPulse Infrastructure Health Report"
		headers = []string{"Server", "Hostname", "Environment", "Group", "OS", "Status", "Availability", "CPU %", "Memory %", "Disk %"}
		for _, srv := range rep.Servers {
			rows = append(rows, []string{
				srv.Name, srv.Hostname, srv.Environment, srv.ServerGroup, srv.OperatingSystem,
				srv.Status, srv.Availability, fmtFloatPtr(srv.CPUPercent), fmtFloatPtr(srv.MemoryPercent), fmtFloatPtr(srv.DiskPercent),
			})
		}
	}

	filename := fmt.Sprintf("cloudpulse-%s-report-%s", repType, time.Now().UTC().Format("20060102-150405"))
	switch format {
	case "json":
		data, err := reports.ExportJSON(rawPayload)
		if err != nil {
			response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "JSON export failed")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.json"`, filename))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	case "html":
		data := reports.ExportPrintableHTML(title, "Range: "+rangeKey, headers, rows)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	default:
		data, err := reports.ExportCSV(headers, rows)
		if err != nil {
			response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "CSV export failed")
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, filename))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

// System Health & Integrations Handlers

func (a *API) handleHealthz(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]any{
		"status": "alive",
		"time":   time.Now().UTC(),
	}, "CloudPulse API is live")
}

func (a *API) handleReadyz(w http.ResponseWriter, r *http.Request) {
	dbErr := a.store.Ping(r.Context())
	promErr := a.prom.Healthy(r.Context())

	status := http.StatusOK
	ready := true
	if dbErr != nil {
		status = http.StatusServiceUnavailable
		ready = false
	}

	response.JSON(w, r, status, map[string]any{
		"ready":      ready,
		"postgres":   healthLabel(dbErr),
		"prometheus": healthLabel(promErr),
		"note":       "PostgreSQL is required for API readiness; temporary Prometheus outages degrade monitoring endpoints without crashing the API.",
	}, "Readiness status")
}

func (a *API) handleSystemVersion(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]any{
		"name":        "CloudPulse",
		"version":     a.cfg.Version,
		"environment": a.cfg.AppEnv,
	}, "Version info")
}

func (a *API) handleSystemIntegrations(w http.ResponseWriter, r *http.Request) {
	dbErr := a.store.Ping(r.Context())
	promErr := a.prom.Healthy(r.Context())
	amErr := a.am.Healthy(r.Context())
	var gfErr error
	var gfLinks any
	gfPublicURL := ""
	if a.grafana != nil {
		gfErr = a.grafana.Healthy(r.Context())
		gfLinks = a.grafana.DashboardLinks("", "")
		gfPublicURL = a.grafana.PublicURL()
	}

	response.JSON(w, r, http.StatusOK, map[string]any{
		"postgres": map[string]any{
			"status":     healthLabel(dbErr),
			"diagnostic": errMsgOrOK(dbErr, "Connected to PostgreSQL metadata store"),
		},
		"prometheus": map[string]any{
			"status":     healthLabel(promErr),
			"url":        a.cfg.PrometheusURL,
			"diagnostic": errMsgOrOK(promErr, "Prometheus TSDB reachable and healthy"),
		},
		"alertmanager": map[string]any{
			"status":     healthLabel(amErr),
			"url":        a.cfg.AlertmanagerURL,
			"diagnostic": errMsgOrOK(amErr, "Alertmanager routing engine reachable"),
		},
		"grafana": map[string]any{
			"status":     healthLabel(gfErr),
			"public_url": gfPublicURL,
			"dashboards": gfLinks,
			"diagnostic": errMsgOrOK(gfErr, "Grafana reachable with provisioned CloudPulse dashboards"),
		},
	}, "Integration health checked")
}

func healthLabel(err error) string {
	if err != nil {
		return "unavailable"
	}
	return "healthy"
}

func errMsgOrOK(err error, okMsg string) string {
	if err != nil {
		return err.Error()
	}
	return okMsg
}

func fmtFloatPtr(v *float64) string {
	if v == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.2f", *v)
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
