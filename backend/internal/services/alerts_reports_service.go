package services

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cloudpulse/cloudpulse-go/backend/internal/models"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/monitoring"
	prom "github.com/cloudpulse/cloudpulse-go/backend/internal/prometheus"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/repository"
)

// MetricCatalogItem describes one metric exposed in the Metrics Explorer catalog (PRD Section 15.1).
type MetricCatalogItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Unit        string `json:"unit"`
	Description string `json:"description"`
	BasePromQL  string `json:"base_promql"`
	MetricType  string `json:"metric_type"` // gauge, counter_rate
}

func SupportedMetricCatalog() []MetricCatalogItem {
	return []MetricCatalogItem{
		{ID: "cpu_usage", Name: "CPU Usage", Category: "CPU", Unit: "%", Description: "Aggregate host CPU utilization percentage", BasePromQL: "cloudpulse_cpu_usage_percent", MetricType: "gauge"},
		{ID: "cpu_load_1m", Name: "CPU Load (1m)", Category: "CPU", Unit: "load", Description: "1-minute system load average where supported", BasePromQL: "cloudpulse_cpu_load_1m", MetricType: "gauge"},
		{ID: "memory_usage", Name: "Memory Usage", Category: "Memory", Unit: "%", Description: "Host memory utilization percentage", BasePromQL: "cloudpulse_memory_usage_percent", MetricType: "gauge"},
		{ID: "memory_available", Name: "Available Memory", Category: "Memory", Unit: "bytes", Description: "OS-reported available host memory in bytes", BasePromQL: "cloudpulse_memory_available_bytes", MetricType: "gauge"},
		{ID: "disk_usage", Name: "Disk Usage", Category: "Disk", Unit: "%", Description: "Disk utilization percentage by mountpoint", BasePromQL: "cloudpulse_disk_usage_percent", MetricType: "gauge"},
		{ID: "disk_free", Name: "Disk Free Space", Category: "Disk", Unit: "bytes", Description: "Available disk space in bytes by mountpoint", BasePromQL: "cloudpulse_disk_free_bytes", MetricType: "gauge"},
		{ID: "net_rx_rate", Name: "Network Receive Rate", Category: "Network", Unit: "B/s", Description: "5-minute receive throughput rate in bytes per second", BasePromQL: "rate(cloudpulse_network_receive_bytes_total[5m])", MetricType: "counter_rate"},
		{ID: "net_tx_rate", Name: "Network Transmit Rate", Category: "Network", Unit: "B/s", Description: "5-minute transmit throughput rate in bytes per second", BasePromQL: "rate(cloudpulse_network_transmit_bytes_total[5m])", MetricType: "counter_rate"},
		{ID: "net_packet_errors", Name: "Packet Errors Rate", Category: "Network", Unit: "err/s", Description: "Combined receive and transmit packet error rate", BasePromQL: "rate(cloudpulse_network_receive_errors_total[5m]) + rate(cloudpulse_network_transmit_errors_total[5m])", MetricType: "counter_rate"},
		{ID: "service_up", Name: "Service Availability", Category: "Services", Unit: "state", Description: "HTTP/TCP service check availability (1=UP, 0=DOWN)", BasePromQL: "cloudpulse_service_up", MetricType: "gauge"},
		{ID: "service_latency", Name: "Service Response Time", Category: "Services", Unit: "s", Description: "HTTP/TCP service check response duration in seconds", BasePromQL: "cloudpulse_service_response_duration_seconds", MetricType: "gauge"},
		{ID: "agent_health", Name: "Agent Health", Category: "Agent", Unit: "state", Description: "Prometheus scrape target reachability (up)", BasePromQL: `up{job="cloudpulse-agents"}`, MetricType: "gauge"},
	}
}

type ExplorerQueryResult struct {
	PrometheusAvailable bool                 `json:"prometheus_available"`
	Query               string               `json:"query"`
	Range               string               `json:"range"`
	Start               time.Time            `json:"start"`
	End                 time.Time            `json:"end"`
	Series              []ExplorerSeries     `json:"series"`
	Summary             ExplorerSummaryStats `json:"summary"`
	MissingDataPoints   int                  `json:"missing_data_points"`
	Error               string               `json:"error,omitempty"`
}

type ExplorerSeries struct {
	Label  string            `json:"label"`
	Metric map[string]string `json:"metric"`
	Points []SimpleDataPoint `json:"points"`
}

type ExplorerSummaryStats struct {
	Min   *float64 `json:"min"`
	Max   *float64 `json:"max"`
	Avg   *float64 `json:"avg"`
	First *float64 `json:"first"`
	Last  *float64 `json:"last"`
	Count int      `json:"count"`
}

func (s *MonitoringService) ExecuteMetricsQuery(
	ctx context.Context,
	orgID, userID uuid.UUID,
	rawExpr, metricID, serverID, env, aggregation, rangeKey, startStr, endStr string,
) (ExplorerQueryResult, error) {
	expr := strings.TrimSpace(rawExpr)
	metricName := metricID

	if expr == "" {
		// Build query from Metric Catalog + filters
		base := "cloudpulse_cpu_usage_percent"
		isRate := false
		for _, item := range SupportedMetricCatalog() {
			if item.ID == metricID || item.BasePromQL == metricID {
				base = item.BasePromQL
				metricName = item.Name
				isRate = item.MetricType == "counter_rate"
				break
			}
		}

		var selectors []string
		selectors = append(selectors, prom.OrgFilter(orgID.String()))
		if serverID != "" && prom.ValidateLabelValue(serverID) {
			selectors = append(selectors, prom.ServerFilter(serverID))
		}
		if env != "" && prom.ValidateLabelValue(env) {
			selectors = append(selectors, fmt.Sprintf(`environment="%s"`, prom.EscapeLabelValue(env)))
		}
		sel := prom.CombineSelectors(selectors...)

		if isRate {
			expr = injectSelectorIntoRate(base, sel)
		} else if strings.Contains(base, "{") {
			expr = strings.Replace(base, "}", ","+strings.Trim(sel, "{}")+"}", 1)
		} else {
			expr = base + sel
		}

		switch strings.ToLower(strings.TrimSpace(aggregation)) {
		case "avg", "max", "min", "sum":
			expr = fmt.Sprintf("%s(%s)", strings.ToLower(strings.TrimSpace(aggregation)), expr)
		}
	}

	if err := prom.ValidateControlledPromQL(expr); err != nil {
		return ExplorerQueryResult{Query: expr, Error: err.Error()}, err
	}

	start, end, step := monitoring.ParseTimeRange(rangeKey, startStr, endStr)
	matrix, err := s.prom.QueryRange(ctx, expr, start, end, step)
	if err != nil {
		return ExplorerQueryResult{
			PrometheusAvailable: false,
			Query:               expr,
			Range:               rangeKey,
			Start:               start,
			End:                 end,
			Series:              []ExplorerSeries{},
			Error:               "Prometheus query failed: " + err.Error(),
		}, nil
	}

	_ = s.store.RecordQueryHistory(ctx, orgID, userID, firstNonEmpty(metricName, "PromQL"), expr, firstNonEmpty(rangeKey, "1h"))

	out := ExplorerQueryResult{
		PrometheusAvailable: true,
		Query:               expr,
		Range:               rangeKey,
		Start:               start,
		End:                 end,
		Series:              make([]ExplorerSeries, 0, len(matrix)),
	}

	var minV, maxV, sumV float64
	minV = math.MaxFloat64
	maxV = -math.MaxFloat64
	var firstV, lastV *float64
	totalPts := 0
	expectedPts := int(end.Sub(start)/step) + 1

	for _, ser := range matrix {
		label := buildSeriesLabel(ser.Metric)
		pts := make([]SimpleDataPoint, 0, len(ser.Points))
		for _, p := range ser.Points {
			v := round2(p.Value)
			pts = append(pts, SimpleDataPoint{Timestamp: p.Timestamp, Value: v})
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
			sumV += v
			if firstV == nil {
				fv := v
				firstV = &fv
			}
			lv := v
			lastV = &lv
			totalPts++
		}
		if expectedPts > len(pts) {
			out.MissingDataPoints += expectedPts - len(pts)
		}
		out.Series = append(out.Series, ExplorerSeries{
			Label:  label,
			Metric: ser.Metric,
			Points: pts,
		})
	}

	if totalPts > 0 {
		mn := round2(minV)
		mx := round2(maxV)
		av := round2(sumV / float64(totalPts))
		out.Summary = ExplorerSummaryStats{
			Min:   &mn,
			Max:   &mx,
			Avg:   &av,
			First: firstV,
			Last:  lastV,
			Count: totalPts,
		}
	}

	return out, nil
}

func injectSelectorIntoRate(base, sel string) string {
	if sel == "" {
		return base
	}
	return strings.ReplaceAll(base, "[5m]", sel+"[5m]")
}

func buildSeriesLabel(m map[string]string) string {
	if name := m["server_name"]; name != "" {
		if mp := m["mountpoint"]; mp != "" {
			return name + " (" + mp + ")"
		}
		if iface := m["interface"]; iface != "" {
			return name + " (" + iface + ")"
		}
		if svc := m["service"]; svc != "" {
			return name + " / " + svc
		}
		return name
	}
	if inst := m["instance"]; inst != "" {
		return inst
	}
	return "series"
}

// AlertView combines Alertmanager/Prometheus live alerts with PostgreSQL acknowledgements.
type AlertView struct {
	Fingerprint     string                       `json:"fingerprint"`
	Title           string                       `json:"title"`
	AlertName       string                       `json:"alert_name"`
	Severity        string                       `json:"severity"`
	Description     string                       `json:"description"`
	ServerID        string                       `json:"server_id"`
	ServerName      string                       `json:"server_name"`
	Environment     string                       `json:"environment"`
	State           string                       `json:"state"` // active, pending, resolved
	TriggeredAt     time.Time                    `json:"triggered_at"`
	UpdatedAt       time.Time                    `json:"updated_at"`
	Labels          map[string]string            `json:"labels"`
	Annotations     map[string]string            `json:"annotations"`
	Acknowledged    bool                         `json:"acknowledged"`
	Acknowledgement *models.AlertAcknowledgement `json:"acknowledgement,omitempty"`
}

type AlertFilter struct {
	Severity     string
	Server       string
	Environment  string
	State        string // active, resolved, all
	Acknowledged string // true, false, all
}

func (s *MonitoringService) ListActiveAlerts(ctx context.Context, orgID uuid.UUID, f AlertFilter) ([]AlertView, error) {
	acks, _ := s.store.ListAcknowledgements(ctx, orgID)
	orgStr := orgID.String()

	byFP := make(map[string]AlertView)

	// 1. Fetch from Alertmanager (primary alerting source)
	if s.am != nil {
		if amAlerts, err := s.am.Alerts(ctx); err == nil {
			for _, a := range amAlerts {
				if lblOrg := a.Labels["cloudpulse_org_id"]; lblOrg != "" && lblOrg != orgStr {
					continue
				}
				fp := a.Fingerprint
				if fp == "" {
					fp = computeFingerprint(a.Labels)
				}
				state := "active"
				if !a.EndsAt.IsZero() && a.EndsAt.Before(time.Now().UTC()) {
					state = "resolved"
				}
				av := AlertView{
					Fingerprint: fp,
					AlertName:   firstNonEmpty(a.Labels["alertname"], "Alert"),
					Title:       firstNonEmpty(a.Annotations["summary"], a.Labels["alertname"], "Infrastructure Alert"),
					Severity:    strings.ToLower(firstNonEmpty(a.Labels["severity"], "warning")),
					Description: firstNonEmpty(a.Annotations["description"], "Alert threshold triggered in Prometheus"),
					ServerID:    a.Labels["cloudpulse_server_id"],
					ServerName:  firstNonEmpty(a.Labels["server_name"], a.Labels["instance"], "Unknown"),
					Environment: firstNonEmpty(a.Labels["environment"], "unknown"),
					State:       state,
					TriggeredAt: a.StartsAt,
					UpdatedAt:   a.UpdatedAt,
					Labels:      a.Labels,
					Annotations: a.Annotations,
				}
				byFP[fp] = av
			}
		}
	}

	// 2. Also include Prometheus firing/pending alerts not yet flushed to Alertmanager
	if s.prom != nil {
		if pAlerts, err := s.prom.Alerts(ctx); err == nil {
			for _, a := range pAlerts {
				if lblOrg := a.Labels["cloudpulse_org_id"]; lblOrg != "" && lblOrg != orgStr {
					continue
				}
				fp := computeFingerprint(a.Labels)
				if _, exists := byFP[fp]; exists {
					continue
				}
				triggered := time.Now().UTC()
				if a.ActiveAt != nil {
					triggered = *a.ActiveAt
				}
				state := "active"
				if a.State == "pending" {
					state = "pending"
				}
				byFP[fp] = AlertView{
					Fingerprint: fp,
					AlertName:   firstNonEmpty(a.Labels["alertname"], "Alert"),
					Title:       firstNonEmpty(a.Annotations["summary"], a.Labels["alertname"], "Infrastructure Alert"),
					Severity:    strings.ToLower(firstNonEmpty(a.Labels["severity"], "warning")),
					Description: firstNonEmpty(a.Annotations["description"], "Alert condition detected by Prometheus"),
					ServerID:    a.Labels["cloudpulse_server_id"],
					ServerName:  firstNonEmpty(a.Labels["server_name"], a.Labels["instance"], "Unknown"),
					Environment: firstNonEmpty(a.Labels["environment"], "unknown"),
					State:       state,
					TriggeredAt: triggered,
					UpdatedAt:   time.Now().UTC(),
					Labels:      a.Labels,
					Annotations: a.Annotations,
				}
			}
		}
	}

	out := make([]AlertView, 0, len(byFP))
	for _, a := range byFP {
		if ack, ok := acks[a.Fingerprint]; ok {
			ackCopy := ack
			a.Acknowledged = true
			a.Acknowledgement = &ackCopy
		}

		if f.Severity != "" && !strings.EqualFold(a.Severity, f.Severity) {
			continue
		}
		if f.Environment != "" && !strings.EqualFold(a.Environment, f.Environment) {
			continue
		}
		if f.Server != "" && !strings.Contains(strings.ToLower(a.ServerName), strings.ToLower(f.Server)) && a.ServerID != f.Server {
			continue
		}
		if f.State != "" && f.State != "all" {
			if f.State == "active" && a.State == "resolved" {
				continue
			}
			if f.State == "resolved" && a.State != "resolved" {
				continue
			}
		}
		if f.Acknowledged == "true" && !a.Acknowledged {
			continue
		}
		if f.Acknowledged == "false" && a.Acknowledged {
			continue
		}

		out = append(out, a)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].TriggeredAt.After(out[j].TriggeredAt)
	})
	return out, nil
}

type AlertDetailResponse struct {
	Alert              AlertView                     `json:"alert"`
	Acknowledgements   []models.AlertAcknowledgement `json:"acknowledgements"`
	RelatedMetrics     []SimpleDataPoint             `json:"related_metrics"`
	RelatedMetricLabel string                        `json:"related_metric_label"`
	Timeline           []AlertTimelineEvent          `json:"timeline"`
}

type AlertTimelineEvent struct {
	Event     string    `json:"event"`
	Actor     string    `json:"actor"`
	Detail    string    `json:"detail"`
	Timestamp time.Time `json:"timestamp"`
}

func (s *MonitoringService) GetAlertDetail(ctx context.Context, orgID uuid.UUID, fingerprint string) (AlertDetailResponse, bool, error) {
	alerts, err := s.ListActiveAlerts(ctx, orgID, AlertFilter{State: "all"})
	if err != nil {
		return AlertDetailResponse{}, false, err
	}

	var found *AlertView
	for i := range alerts {
		if alerts[i].Fingerprint == fingerprint {
			found = &alerts[i]
			break
		}
	}

	ackHistory, _ := s.store.ListAcknowledgementHistory(ctx, orgID, fingerprint)
	if found == nil {
		if len(ackHistory) == 0 {
			return AlertDetailResponse{}, false, nil
		}
		// Resolved alert with acknowledgement history
		found = &AlertView{
			Fingerprint:     fingerprint,
			Title:           "Resolved Alert (" + fingerprint + ")",
			AlertName:       "ResolvedAlert",
			Severity:        "resolved",
			Description:     "This alert is no longer firing in Prometheus / Alertmanager.",
			State:           "resolved",
			TriggeredAt:     ackHistory[len(ackHistory)-1].AcknowledgedAt,
			UpdatedAt:       ackHistory[0].AcknowledgedAt,
			Labels:          map[string]string{},
			Annotations:     map[string]string{},
			Acknowledged:    true,
			Acknowledgement: &ackHistory[0],
		}
	}

	timeline := []AlertTimelineEvent{
		{
			Event:     "Alert Triggered",
			Actor:     "Prometheus",
			Detail:    found.Description,
			Timestamp: found.TriggeredAt,
		},
	}
	for _, ack := range ackHistory {
		timeline = append(timeline, AlertTimelineEvent{
			Event:     "Alert Acknowledged",
			Actor:     firstNonEmpty(ack.AcknowledgedName, "Operator"),
			Detail:    firstNonEmpty(ack.Note, "Acknowledged in CloudPulse (Prometheus alert remains active until condition resolves)"),
			Timestamp: ack.AcknowledgedAt,
		})
	}
	sort.Slice(timeline, func(i, j int) bool { return timeline[i].Timestamp.Before(timeline[j].Timestamp) })

	relatedPts := []SimpleDataPoint{}
	relatedLabel := "CPU Usage (%)"
	if s.prom != nil && found.ServerID != "" {
		now := time.Now().UTC()
		query := prom.QueryCPUUsage(orgID.String(), found.ServerID)
		nameLower := strings.ToLower(found.AlertName + " " + found.Title)
		switch {
		case strings.Contains(nameLower, "memory"):
			query = prom.QueryMemoryUsage(orgID.String(), found.ServerID)
			relatedLabel = "Memory Usage (%)"
		case strings.Contains(nameLower, "disk"):
			query = "max(" + prom.QueryDiskUsage(orgID.String(), found.ServerID) + ")"
			relatedLabel = "Disk Usage (%)"
		case strings.Contains(nameLower, "down") || strings.Contains(nameLower, "server"):
			query = prom.QueryAvailability(orgID.String(), found.ServerID)
			relatedLabel = "Target Availability (up)"
		}
		if ser, err := s.prom.QueryRange(ctx, query, now.Add(-time.Hour), now, 30*time.Second); err == nil {
			relatedPts = extractFirstSeries(ser)
		}
	}

	return AlertDetailResponse{
		Alert:              *found,
		Acknowledgements:   ackHistory,
		RelatedMetrics:     relatedPts,
		RelatedMetricLabel: relatedLabel,
		Timeline:           timeline,
	}, true, nil
}

// Reports generation (PRD Section 16)
type InfrastructureReport struct {
	GeneratedAt         time.Time       `json:"generated_at"`
	Range               string          `json:"range"`
	TotalServers        int             `json:"total_servers"`
	UpServers           int             `json:"up_servers"`
	DownServers         int             `json:"down_servers"`
	AvailabilityPercent *float64        `json:"availability_percent"`
	ActiveAlerts        int             `json:"active_alerts"`
	HealthDistribution  map[string]int  `json:"health_distribution"`
	Servers             []models.Server `json:"servers"`
}

type ResourceReportRow struct {
	ServerID        string   `json:"server_id"`
	ServerName      string   `json:"server_name"`
	Environment     string   `json:"environment"`
	ServerGroup     string   `json:"server_group"`
	AvgCPU          *float64 `json:"avg_cpu"`
	PeakCPU         *float64 `json:"peak_cpu"`
	AvgMemory       *float64 `json:"avg_memory"`
	PeakMemory      *float64 `json:"peak_memory"`
	DiskUtilization *float64 `json:"disk_utilization"`
	NetworkRxBps    *float64 `json:"network_rx_bps"`
	NetworkTxBps    *float64 `json:"network_tx_bps"`
}

type ResourceReport struct {
	GeneratedAt time.Time           `json:"generated_at"`
	Range       string              `json:"range"`
	Rows        []ResourceReportRow `json:"rows"`
}

type ServiceReport struct {
	GeneratedAt time.Time             `json:"generated_at"`
	Range       string                `json:"range"`
	Services    []models.ServiceCheck `json:"services"`
}

type AlertHistoryReport struct {
	GeneratedAt         time.Time      `json:"generated_at"`
	Range               string         `json:"range"`
	TotalAlerts         int            `json:"total_alerts"`
	CriticalAlerts      int            `json:"critical_alerts"`
	WarningAlerts       int            `json:"warning_alerts"`
	ResolvedAlerts      int            `json:"resolved_alerts"`
	MostAffectedServers map[string]int `json:"most_affected_servers"`
	Alerts              []AlertView    `json:"alerts"`
}

func (s *MonitoringService) BuildInfrastructureReport(ctx context.Context, orgID uuid.UUID, env, group, rangeKey string) (InfrastructureReport, error) {
	servers, _, err := s.store.ListServers(ctx, orgID, repository.ServerFilter{
		Environment: env,
		ServerGroup: group,
		Page:        1,
		PageSize:    100,
	})
	if err != nil {
		return InfrastructureReport{}, err
	}
	servers, _ = s.EnrichServers(ctx, orgID, servers)
	alerts, _ := s.ListActiveAlerts(ctx, orgID, AlertFilter{Environment: env, State: "active"})

	rep := InfrastructureReport{
		GeneratedAt:        time.Now().UTC(),
		Range:              firstNonEmpty(rangeKey, "24h"),
		TotalServers:       len(servers),
		ActiveAlerts:       len(alerts),
		HealthDistribution: map[string]int{},
		Servers:            servers,
	}
	for _, srv := range servers {
		rep.HealthDistribution[srv.Status]++
		if srv.Availability == models.StatusUp {
			rep.UpServers++
		} else if srv.Availability == models.StatusDown {
			rep.DownServers++
		}
	}
	if rep.UpServers+rep.DownServers > 0 {
		pct := round2(float64(rep.UpServers) / float64(rep.UpServers+rep.DownServers) * 100)
		rep.AvailabilityPercent = &pct
	}
	return rep, nil
}

func (s *MonitoringService) BuildResourceReport(ctx context.Context, orgID uuid.UUID, env, group, rangeKey string) (ResourceReport, error) {
	servers, _, err := s.store.ListServers(ctx, orgID, repository.ServerFilter{
		Environment: env,
		ServerGroup: group,
		Page:        1,
		PageSize:    100,
	})
	if err != nil {
		return ResourceReport{}, err
	}
	servers, promOnline := s.EnrichServers(ctx, orgID, servers)
	start, end, step := monitoring.ParseTimeRange(firstNonEmpty(rangeKey, "24h"), "", "")
	orgStr := orgID.String()

	rows := make([]ResourceReportRow, 0, len(servers))
	for _, srv := range servers {
		row := ResourceReportRow{
			ServerID:        srv.ID.String(),
			ServerName:      srv.Name,
			Environment:     srv.Environment,
			ServerGroup:     srv.ServerGroup,
			AvgCPU:          srv.CPUPercent,
			PeakCPU:         srv.CPUPercent,
			AvgMemory:       srv.MemoryPercent,
			PeakMemory:      srv.MemoryPercent,
			DiskUtilization: srv.DiskPercent,
		}
		if promOnline {
			sid := srv.ID.String()
			if ser, err := s.prom.QueryRange(ctx, prom.QueryCPUUsage(orgStr, sid), start, end, step); err == nil && len(ser) > 0 && len(ser[0].Points) > 0 {
				avg, pk := seriesAvgPeak(ser[0].Points)
				row.AvgCPU, row.PeakCPU = &avg, &pk
			}
			if ser, err := s.prom.QueryRange(ctx, prom.QueryMemoryUsage(orgStr, sid), start, end, step); err == nil && len(ser) > 0 && len(ser[0].Points) > 0 {
				avg, pk := seriesAvgPeak(ser[0].Points)
				row.AvgMemory, row.PeakMemory = &avg, &pk
			}
			if sp, err := s.prom.QueryInstant(ctx, "sum("+prom.QueryNetworkReceiveRate(orgStr, sid)+")", end); err == nil && len(sp) > 0 {
				v := round2(sp[0].Value)
				row.NetworkRxBps = &v
			}
			if sp, err := s.prom.QueryInstant(ctx, "sum("+prom.QueryNetworkTransmitRate(orgStr, sid)+")", end); err == nil && len(sp) > 0 {
				v := round2(sp[0].Value)
				row.NetworkTxBps = &v
			}
		}
		rows = append(rows, row)
	}

	return ResourceReport{
		GeneratedAt: time.Now().UTC(),
		Range:       firstNonEmpty(rangeKey, "24h"),
		Rows:        rows,
	}, nil
}

func (s *MonitoringService) BuildServiceReport(ctx context.Context, orgID uuid.UUID, rangeKey string) (ServiceReport, error) {
	svcs, err := s.ListServicesEnriched(ctx, orgID, nil)
	if err != nil {
		return ServiceReport{}, err
	}
	return ServiceReport{
		GeneratedAt: time.Now().UTC(),
		Range:       firstNonEmpty(rangeKey, "24h"),
		Services:    svcs,
	}, nil
}

func (s *MonitoringService) BuildAlertReport(ctx context.Context, orgID uuid.UUID, env, severity, rangeKey string) (AlertHistoryReport, error) {
	alerts, err := s.ListActiveAlerts(ctx, orgID, AlertFilter{Environment: env, Severity: severity, State: "all"})
	if err != nil {
		return AlertHistoryReport{}, err
	}
	rep := AlertHistoryReport{
		GeneratedAt:         time.Now().UTC(),
		Range:               firstNonEmpty(rangeKey, "24h"),
		TotalAlerts:         len(alerts),
		MostAffectedServers: map[string]int{},
		Alerts:              alerts,
	}
	for _, a := range alerts {
		switch strings.ToLower(a.Severity) {
		case "critical":
			rep.CriticalAlerts++
		case "warning":
			rep.WarningAlerts++
		}
		if a.State == "resolved" {
			rep.ResolvedAlerts++
		}
		if a.ServerName != "" {
			rep.MostAffectedServers[a.ServerName]++
		}
	}
	return rep, nil
}

func seriesAvgPeak(pts []prom.DataPoint) (float64, float64) {
	sum := 0.0
	peak := 0.0
	for _, p := range pts {
		sum += p.Value
		if p.Value > peak {
			peak = p.Value
		}
	}
	return round2(sum / float64(len(pts))), round2(peak)
}
