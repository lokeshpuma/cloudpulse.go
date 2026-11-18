package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cloudpulse/cloudpulse-go/backend/internal/alertmanager"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/discovery"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/grafana"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/models"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/monitoring"
	prom "github.com/cloudpulse/cloudpulse-go/backend/internal/prometheus"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/repository"
)

// MonitoringService orchestrates live Prometheus queries, Alertmanager status,
// discovery synchronization, and PostgreSQL metadata without duplicating time series in Postgres.
type MonitoringService struct {
	store     repository.Store
	prom      prom.Client
	am        alertmanager.Client
	grafana   *grafana.Client
	discovery *discovery.Manager
	logger    *slog.Logger
}

func NewMonitoringService(
	store repository.Store,
	promClient prom.Client,
	amClient alertmanager.Client,
	gfClient *grafana.Client,
	disc *discovery.Manager,
	logger *slog.Logger,
) *MonitoringService {
	if logger == nil {
		logger = slog.Default()
	}
	return &MonitoringService{
		store:     store,
		prom:      promClient,
		am:        amClient,
		grafana:   gfClient,
		discovery: disc,
		logger:    logger,
	}
}

// SyncDiscovery rebuilds Prometheus file_sd targets from all enabled servers.
func (s *MonitoringService) SyncDiscovery(ctx context.Context) error {
	if s.discovery == nil {
		return nil
	}
	servers, err := s.store.ListAllEnabledServers(ctx)
	if err != nil {
		return err
	}
	return s.discovery.SyncTargets(ctx, servers)
}

// SyncOrgAlertRules writes the organization's alert rules to Prometheus rules directory.
func (s *MonitoringService) SyncOrgAlertRules(ctx context.Context, orgID uuid.UUID) error {
	if s.discovery == nil {
		return nil
	}
	rules, err := s.store.ListAlertRules(ctx, orgID)
	if err != nil {
		return err
	}
	return s.discovery.SyncAlertRules(ctx, orgID.String(), rules)
}

// EnrichServers populates real-time status, CPU, memory, disk, and freshness from Prometheus.
func (s *MonitoringService) EnrichServers(ctx context.Context, orgID uuid.UUID, servers []models.Server) ([]models.Server, bool) {
	if len(servers) == 0 {
		return servers, true
	}

	now := time.Now().UTC()
	orgStr := orgID.String()

	upSamples, errUp := s.prom.QueryInstant(ctx, prom.QueryAvailability(orgStr, ""), now)
	promOnline := errUp == nil

	var cpuSamples, memSamples, diskSamples, buildSamples []prom.Sample
	var targets []prom.Target
	var promAlerts []prom.PromAlert

	if promOnline {
		cpuSamples, _ = s.prom.QueryInstant(ctx, prom.QueryCPUUsage(orgStr, ""), now)
		memSamples, _ = s.prom.QueryInstant(ctx, prom.QueryMemoryUsage(orgStr, ""), now)
		diskSamples, _ = s.prom.QueryInstant(ctx, prom.QueryDiskMaxByServer(orgStr), now)
		buildSamples, _ = s.prom.QueryInstant(ctx, prom.QueryAgentBuildInfo(orgStr, ""), now)
		targets, _ = s.prom.Targets(ctx)
		promAlerts, _ = s.prom.Alerts(ctx)
	}

	upBySrv := indexSamplesByServer(upSamples)
	cpuBySrv := indexSamplesByServer(cpuSamples)
	memBySrv := indexSamplesByServer(memSamples)
	diskBySrv := indexSamplesByServer(diskSamples)
	buildBySrv := indexSamplesByServer(buildSamples)

	targetBySrv := make(map[string]prom.Target)
	for _, t := range targets {
		if sid := t.Labels["cloudpulse_server_id"]; sid != "" {
			targetBySrv[sid] = t
		}
	}

	critAlertBySrv := make(map[string]bool)
	warnAlertBySrv := make(map[string]bool)
	for _, a := range promAlerts {
		if a.State != "firing" {
			continue
		}
		sid := a.Labels["cloudpulse_server_id"]
		if sid == "" {
			continue
		}
		switch strings.ToLower(a.Labels["severity"]) {
		case "critical":
			critAlertBySrv[sid] = true
		case "warning":
			warnAlertBySrv[sid] = true
		}
	}

	for i := range servers {
		sid := servers[i].ID.String()
		var upVal *float64
		var sampleAt *time.Time
		if sp, ok := upBySrv[sid]; ok {
			v := sp.Value
			upVal = &v
			t := sp.Timestamp
			sampleAt = &t
		}
		if sp, ok := cpuBySrv[sid]; ok {
			v := round2(sp.Value)
			servers[i].CPUPercent = &v
		}
		if sp, ok := memBySrv[sid]; ok {
			v := round2(sp.Value)
			servers[i].MemoryPercent = &v
		}
		if sp, ok := diskBySrv[sid]; ok {
			v := round2(sp.Value)
			servers[i].DiskPercent = &v
		}

		targetHealth := ""
		if tgt, ok := targetBySrv[sid]; ok {
			targetHealth = strings.ToLower(tgt.Health)
			if !tgt.LastScrape.IsZero() && (sampleAt == nil || tgt.LastScrape.After(*sampleAt)) {
				t := tgt.LastScrape
				sampleAt = &t
			}
		}

		// Persist heartbeat metadata when target is successfully reporting
		if upVal != nil && *upVal == 1 && sampleAt != nil {
			agentVer := servers[i].AgentVersion
			osName := servers[i].OperatingSystem
			arch := servers[i].Architecture
			if b, ok := buildBySrv[sid]; ok {
				if v := b.Metric["version"]; v != "" {
					agentVer = v
					servers[i].AgentVersion = v
				}
				if v := b.Metric["os"]; v != "" {
					osName = v
					servers[i].OperatingSystem = v
				}
				if v := b.Metric["arch"]; v != "" {
					arch = v
					servers[i].Architecture = v
				}
			}
			servers[i].LastSeenAt = sampleAt
			_ = s.store.UpdateServerHeartbeat(ctx, servers[i].ID, agentVer, osName, arch, *sampleAt)
		}

		derived := monitoring.DeriveServerStatus(monitoring.ServerLiveInput{
			MonitoringEnabled: servers[i].MonitoringEnabled,
			LastSeenAt:        servers[i].LastSeenAt,
			TargetHealth:      targetHealth,
			UpSample:          upVal,
			LastSampleAt:      sampleAt,
			CPU:               servers[i].CPUPercent,
			Memory:            servers[i].MemoryPercent,
			DiskMax:           servers[i].DiskPercent,
			HasCriticalAlert:  critAlertBySrv[sid],
			HasWarningAlert:   warnAlertBySrv[sid],
			PrometheusOnline:  promOnline,
			Now:               now,
		})

		servers[i].Status = derived.Status
		servers[i].Availability = derived.Availability
		servers[i].IsStale = derived.IsStale
		servers[i].LastSampleAt = sampleAt
		servers[i].DataSource = derived.DataSource
	}

	return servers, promOnline
}

// OverviewResponse backs the 6 KPI cards and overview widgets on the Main Dashboard.
type OverviewResponse struct {
	PrometheusAvailable bool                    `json:"prometheus_available"`
	DataSource          string                  `json:"data_source"`
	AggregationMethod   string                  `json:"aggregation_method"`
	GeneratedAt         time.Time               `json:"generated_at"`
	Servers             ServerKPICard           `json:"servers"`
	Health              HealthKPICard           `json:"health"`
	CPU                 CPUKPICard              `json:"cpu"`
	Memory              MemoryKPICard           `json:"memory"`
	Disk                DiskKPICard             `json:"disk"`
	Alerts              AlertsKPICard           `json:"alerts"`
	RecentEvents        []InfrastructureEvent   `json:"recent_events"`
	GrafanaDashboards   []grafana.DashboardLink `json:"grafana_dashboards"`
}

type ServerKPICard struct {
	Total               int      `json:"total"`
	Up                  int      `json:"up"`
	Down                int      `json:"down"`
	Unknown             int      `json:"unknown"`
	Pending             int      `json:"pending"`
	AvailabilityPercent *float64 `json:"availability_percent"`
}

type HealthKPICard struct {
	Healthy  int `json:"healthy"`
	Warning  int `json:"warning"`
	Critical int `json:"critical"`
	Unknown  int `json:"unknown"`
}

type CPUKPICard struct {
	AveragePercent *float64          `json:"average_percent"`
	ChangePercent  *float64          `json:"change_percent"`
	IncludedCount  int               `json:"included_count"`
	Trend          []SimpleDataPoint `json:"trend"`
}

type MemoryKPICard struct {
	AveragePercent *float64 `json:"average_percent"`
	UsedBytes      *float64 `json:"used_bytes"`
	TotalBytes     *float64 `json:"total_bytes"`
	AvailableBytes *float64 `json:"available_bytes"`
	ChangePercent  *float64 `json:"change_percent"`
	IncludedCount  int      `json:"included_count"`
}

type DiskKPICard struct {
	AveragePercent  *float64 `json:"average_percent"`
	HighestServer   string   `json:"highest_server"`
	HighestServerID string   `json:"highest_server_id"`
	HighestPercent  *float64 `json:"highest_percent"`
	CriticalCount   int      `json:"critical_count"`
	IncludedCount   int      `json:"included_count"`
}

type AlertsKPICard struct {
	Total          int `json:"total"`
	Critical       int `json:"critical"`
	Warning        int `json:"warning"`
	Info           int `json:"info"`
	Unacknowledged int `json:"unacknowledged"`
}

type SimpleDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type InfrastructureEvent struct {
	ID           string    `json:"id"`
	Action       string    `json:"action"`
	Title        string    `json:"title"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	Actor        string    `json:"actor"`
	Severity     string    `json:"severity"`
	Timestamp    time.Time `json:"timestamp"`
}

func (s *MonitoringService) GetOverview(ctx context.Context, orgID uuid.UUID, rangeKey string) (OverviewResponse, error) {
	now := time.Now().UTC()
	servers, _, err := s.store.ListServers(ctx, orgID, repository.ServerFilter{Page: 1, PageSize: 100})
	if err != nil {
		return OverviewResponse{}, err
	}
	servers, promOnline := s.EnrichServers(ctx, orgID, servers)

	out := OverviewResponse{
		PrometheusAvailable: promOnline,
		DataSource:          "live_prometheus",
		AggregationMethod:   "unweighted_mean_of_reporting_machines",
		GeneratedAt:         now,
		RecentEvents:        []InfrastructureEvent{},
	}
	if !promOnline {
		out.DataSource = "unavailable"
	}
	if s.grafana != nil {
		out.GrafanaDashboards = s.grafana.DashboardLinks("", "")
	}

	var cpuVals, memVals, diskVals []*float64
	var highestDiskVal float64 = -1
	var highestDiskSrv, highestDiskID string
	critDisks := 0

	for _, srv := range servers {
		out.Servers.Total++
		switch srv.Availability {
		case models.StatusUp:
			out.Servers.Up++
		case models.StatusDown:
			out.Servers.Down++
		default:
			if srv.Status == models.StatusPending {
				out.Servers.Pending++
			} else {
				out.Servers.Unknown++
			}
		}

		switch srv.Status {
		case models.StatusUp:
			out.Health.Healthy++
		case models.StatusWarning:
			out.Health.Warning++
		case models.StatusCritical, models.StatusDown:
			out.Health.Critical++
		default:
			out.Health.Unknown++
		}

		if srv.CPUPercent != nil {
			cpuVals = append(cpuVals, srv.CPUPercent)
		}
		if srv.MemoryPercent != nil {
			memVals = append(memVals, srv.MemoryPercent)
		}
		if srv.DiskPercent != nil {
			diskVals = append(diskVals, srv.DiskPercent)
			if *srv.DiskPercent > highestDiskVal {
				highestDiskVal = *srv.DiskPercent
				highestDiskSrv = srv.Name
				highestDiskID = srv.ID.String()
			}
			if *srv.DiskPercent >= monitoring.DefaultDiskCrit {
				critDisks++
			}
		}
	}

	monitoredActive := out.Servers.Up + out.Servers.Down
	if monitoredActive > 0 {
		pct := round2(float64(out.Servers.Up) / float64(monitoredActive) * 100)
		out.Servers.AvailabilityPercent = &pct
	}

	out.CPU.AveragePercent, out.CPU.IncludedCount = monitoring.UnweightedAverage(cpuVals)
	out.Memory.AveragePercent, out.Memory.IncludedCount = monitoring.UnweightedAverage(memVals)
	out.Disk.AveragePercent, out.Disk.IncludedCount = monitoring.UnweightedAverage(diskVals)
	out.Disk.CriticalCount = critDisks
	if highestDiskVal >= 0 {
		v := round2(highestDiskVal)
		out.Disk.HighestPercent = &v
		out.Disk.HighestServer = highestDiskSrv
		out.Disk.HighestServerID = highestDiskID
	}

	if promOnline {
		orgStr := orgID.String()
		// Memory byte aggregates
		if usedSamples, err := s.prom.QueryInstant(ctx, prom.QueryMemoryMetric("cloudpulse_memory_used_bytes", orgStr, ""), now); err == nil && len(usedSamples) > 0 {
			sum := sumSamples(usedSamples)
			out.Memory.UsedBytes = &sum
		}
		if totalSamples, err := s.prom.QueryInstant(ctx, prom.QueryMemoryMetric("cloudpulse_memory_total_bytes", orgStr, ""), now); err == nil && len(totalSamples) > 0 {
			sum := sumSamples(totalSamples)
			out.Memory.TotalBytes = &sum
		}
		if availSamples, err := s.prom.QueryInstant(ctx, prom.QueryMemoryMetric("cloudpulse_memory_available_bytes", orgStr, ""), now); err == nil && len(availSamples) > 0 {
			sum := sumSamples(availSamples)
			out.Memory.AvailableBytes = &sum
		}

		// CPU sparkline trend & change over comparison period
		start, end, step := monitoring.ParseTimeRange(rangeKey, "", "")
		cpuQuery := fmt.Sprintf("avg(cloudpulse_cpu_usage_percent%s)", prom.CombineSelectors(prom.OrgFilter(orgStr)))
		if series, err := s.prom.QueryRange(ctx, cpuQuery, start, end, step); err == nil && len(series) > 0 {
			pts := series[0].Points
			for _, p := range pts {
				out.CPU.Trend = append(out.CPU.Trend, SimpleDataPoint{Timestamp: p.Timestamp, Value: round2(p.Value)})
			}
			if len(pts) >= 2 {
				delta := round2(pts[len(pts)-1].Value - pts[0].Value)
				out.CPU.ChangePercent = &delta
			}
		}

		memQuery := fmt.Sprintf("avg(cloudpulse_memory_usage_percent%s)", prom.CombineSelectors(prom.OrgFilter(orgStr)))
		if series, err := s.prom.QueryRange(ctx, memQuery, start, end, step); err == nil && len(series) > 0 && len(series[0].Points) >= 2 {
			pts := series[0].Points
			delta := round2(pts[len(pts)-1].Value - pts[0].Value)
			out.Memory.ChangePercent = &delta
		}
	}

	// Active alerts summary
	alerts, _ := s.ListActiveAlerts(ctx, orgID, AlertFilter{State: "active"})
	for _, a := range alerts {
		out.Alerts.Total++
		switch strings.ToLower(a.Severity) {
		case "critical":
			out.Alerts.Critical++
		case "warning":
			out.Alerts.Warning++
		default:
			out.Alerts.Info++
		}
		if !a.Acknowledged {
			out.Alerts.Unacknowledged++
		}
	}

	// Recent infrastructure events from audit logs
	logs, _, _ := s.store.ListAuditLogs(ctx, orgID, "", "", 1, 12)
	for _, l := range logs {
		out.RecentEvents = append(out.RecentEvents, InfrastructureEvent{
			ID:           l.ID.String(),
			Action:       l.Action,
			Title:        humanizeAction(l.Action, l.ResourceType),
			ResourceType: l.ResourceType,
			ResourceID:   l.ResourceID,
			Actor:        firstNonEmpty(l.ActorName, l.ActorEmail, "System"),
			Severity:     actionSeverity(l.Action),
			Timestamp:    l.CreatedAt,
		})
	}

	return out, nil
}

// InfrastructureHealthResponse backs the Infrastructure Overview page (PRD Section 8.1).
type InfrastructureHealthResponse struct {
	PrometheusAvailable  bool                    `json:"prometheus_available"`
	TotalMachines        int                     `json:"total_machines"`
	ByEnvironment        map[string]int          `json:"by_environment"`
	ByOperatingSystem    map[string]int          `json:"by_operating_system"`
	ByStatus             map[string]int          `json:"by_status"`
	RecentlyAdded        []models.Server         `json:"recently_added"`
	RecentlyDisconnected []models.Server         `json:"recently_disconnected"`
	GrafanaDashboards    []grafana.DashboardLink `json:"grafana_dashboards"`
}

func (s *MonitoringService) GetInfrastructureHealth(ctx context.Context, orgID uuid.UUID) (InfrastructureHealthResponse, error) {
	servers, _, err := s.store.ListServers(ctx, orgID, repository.ServerFilter{Page: 1, PageSize: 100, SortBy: "created_at", SortDir: "desc"})
	if err != nil {
		return InfrastructureHealthResponse{}, err
	}
	servers, promOnline := s.EnrichServers(ctx, orgID, servers)

	resp := InfrastructureHealthResponse{
		PrometheusAvailable:  promOnline,
		TotalMachines:        len(servers),
		ByEnvironment:        map[string]int{},
		ByOperatingSystem:    map[string]int{},
		ByStatus:             map[string]int{},
		RecentlyAdded:        []models.Server{},
		RecentlyDisconnected: []models.Server{},
	}
	if s.grafana != nil {
		resp.GrafanaDashboards = s.grafana.DashboardLinks("", "")
	}

	for i, srv := range servers {
		resp.ByEnvironment[srv.Environment]++
		resp.ByOperatingSystem[srv.OperatingSystem]++
		resp.ByStatus[srv.Status]++
		if i < 5 {
			resp.RecentlyAdded = append(resp.RecentlyAdded, srv)
		}
		if srv.Status == models.StatusDown || (srv.IsStale && srv.LastSeenAt != nil) {
			resp.RecentlyDisconnected = append(resp.RecentlyDisconnected, srv)
		}
	}
	return resp, nil
}

// TimeseriesPoint represents a merged dashboard time point across CPU, memory, and disk.
type TimeseriesPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	CPUPercent    *float64  `json:"cpu_percent"`
	MemoryPercent *float64  `json:"memory_percent"`
	DiskPercent   *float64  `json:"disk_percent"`
}

type TimeseriesResponse struct {
	PrometheusAvailable bool              `json:"prometheus_available"`
	Range               string            `json:"range"`
	Start               time.Time         `json:"start"`
	End                 time.Time         `json:"end"`
	StepSeconds         int               `json:"step_seconds"`
	Points              []TimeseriesPoint `json:"points"`
}

func (s *MonitoringService) GetDashboardTimeseries(ctx context.Context, orgID uuid.UUID, serverID, env, rangeKey, startStr, endStr string) (TimeseriesResponse, error) {
	start, end, step := monitoring.ParseTimeRange(rangeKey, startStr, endStr)
	orgStr := orgID.String()

	envFilter := ""
	if env != "" && prom.ValidateLabelValue(env) {
		envFilter = fmt.Sprintf(`environment="%s"`, prom.EscapeLabelValue(env))
	}
	sel := prom.CombineSelectors(prom.OrgFilter(orgStr), prom.ServerFilter(serverID), envFilter)

	cpuSeries, errCPU := s.prom.QueryRange(ctx, "avg(cloudpulse_cpu_usage_percent"+sel+")", start, end, step)
	if errCPU != nil {
		return TimeseriesResponse{
			PrometheusAvailable: false,
			Range:               rangeKey,
			Start:               start,
			End:                 end,
			StepSeconds:         int(step.Seconds()),
			Points:              []TimeseriesPoint{},
		}, nil
	}
	memSeries, _ := s.prom.QueryRange(ctx, "avg(cloudpulse_memory_usage_percent"+sel+")", start, end, step)
	diskSeries, _ := s.prom.QueryRange(ctx, "avg(cloudpulse_disk_usage_percent"+sel+")", start, end, step)

	merged := make(map[int64]*TimeseriesPoint)
	addSeries := func(series []prom.MatrixSeries, assign func(pt *TimeseriesPoint, v float64)) {
		if len(series) == 0 {
			return
		}
		for _, p := range series[0].Points {
			key := p.Timestamp.Unix()
			pt, ok := merged[key]
			if !ok {
				pt = &TimeseriesPoint{Timestamp: p.Timestamp}
				merged[key] = pt
			}
			assign(pt, round2(p.Value))
		}
	}

	addSeries(cpuSeries, func(pt *TimeseriesPoint, v float64) { pt.CPUPercent = &v })
	addSeries(memSeries, func(pt *TimeseriesPoint, v float64) { pt.MemoryPercent = &v })
	addSeries(diskSeries, func(pt *TimeseriesPoint, v float64) { pt.DiskPercent = &v })

	keys := make([]int64, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	points := make([]TimeseriesPoint, 0, len(keys))
	for _, k := range keys {
		points = append(points, *merged[k])
	}

	return TimeseriesResponse{
		PrometheusAvailable: true,
		Range:               rangeKey,
		Start:               start,
		End:                 end,
		StepSeconds:         int(step.Seconds()),
		Points:              points,
	}, nil
}

// ServerDetailMetrics backs Sections A-H of the Server Detail Page (PRD Section 8.4).
type ServerDetailMetrics struct {
	PrometheusAvailable bool                     `json:"prometheus_available"`
	Server              models.Server            `json:"server"`
	CPU                 ServerCPUAnalytics       `json:"cpu"`
	Memory              ServerMemoryAnalytics    `json:"memory"`
	Disks               []DiskMountMetric        `json:"disks"`
	Networks            []NetworkInterfaceMetric `json:"networks"`
	Services            []models.ServiceCheck    `json:"services"`
	RecentActivity      []InfrastructureEvent    `json:"recent_activity"`
	GrafanaDashboards   []grafana.DashboardLink  `json:"grafana_dashboards"`
}

type ServerCPUAnalytics struct {
	UsagePercent      *float64           `json:"usage_percent"`
	CoreCount         *int               `json:"core_count"`
	PerCorePercent    map[string]float64 `json:"per_core_percent"`
	Load1m            *float64           `json:"load_1m"`
	Load5m            *float64           `json:"load_5m"`
	Load15m           *float64           `json:"load_15m"`
	LoadSupported     bool               `json:"load_supported"`
	SaturationWarning bool               `json:"saturation_warning"`
	PeakPercent       *float64           `json:"peak_percent"`
	AveragePercent    *float64           `json:"average_percent"`
}

type ServerMemoryAnalytics struct {
	TotalBytes     *float64 `json:"total_bytes"`
	UsedBytes      *float64 `json:"used_bytes"`
	AvailableBytes *float64 `json:"available_bytes"`
	CachedBytes    *float64 `json:"cached_bytes"`
	FreeBytes      *float64 `json:"free_bytes"`
	UsagePercent   *float64 `json:"usage_percent"`
}

type DiskMountMetric struct {
	Mountpoint   string    `json:"mountpoint"`
	Device       string    `json:"device"`
	Filesystem   string    `json:"filesystem"`
	TotalBytes   float64   `json:"total_bytes"`
	UsedBytes    float64   `json:"used_bytes"`
	FreeBytes    float64   `json:"free_bytes"`
	UsagePercent float64   `json:"usage_percent"`
	CollectedAt  time.Time `json:"collected_at"`
}

type NetworkInterfaceMetric struct {
	Interface           string  `json:"interface"`
	Status              string  `json:"status"`
	ReceiveBytesPerSec  float64 `json:"receive_bytes_per_sec"`
	TransmitBytesPerSec float64 `json:"transmit_bytes_per_sec"`
	TotalReceiveBytes   float64 `json:"total_receive_bytes"`
	TotalTransmitBytes  float64 `json:"total_transmit_bytes"`
	ReceivePackets      float64 `json:"receive_packets_total"`
	TransmitPackets     float64 `json:"transmit_packets_total"`
	ReceiveErrors       float64 `json:"receive_errors_total"`
	TransmitErrors      float64 `json:"transmit_errors_total"`
}

func (s *MonitoringService) GetServerMetrics(ctx context.Context, orgID, serverID uuid.UUID) (ServerDetailMetrics, error) {
	srv, err := s.store.GetServer(ctx, orgID, serverID)
	if err != nil {
		return ServerDetailMetrics{}, err
	}
	enriched, promOnline := s.EnrichServers(ctx, orgID, []models.Server{srv})
	srv = enriched[0]

	out := ServerDetailMetrics{
		PrometheusAvailable: promOnline,
		Server:              srv,
		CPU:                 ServerCPUAnalytics{UsagePercent: srv.CPUPercent, PerCorePercent: map[string]float64{}},
		Memory:              ServerMemoryAnalytics{UsagePercent: srv.MemoryPercent},
		Disks:               []DiskMountMetric{},
		Networks:            []NetworkInterfaceMetric{},
		Services:            []models.ServiceCheck{},
		RecentActivity:      []InfrastructureEvent{},
	}
	if s.grafana != nil {
		out.GrafanaDashboards = s.grafana.DashboardLinks(srv.Name, srv.Environment)
	}

	now := time.Now().UTC()
	orgStr := orgID.String()
	srvStr := serverID.String()

	if promOnline {
		if sp, err := s.prom.QueryInstant(ctx, prom.QueryCPUCoreCount(orgStr, srvStr), now); err == nil && len(sp) > 0 {
			c := int(sp[0].Value)
			out.CPU.CoreCount = &c
		}
		if cores, err := s.prom.QueryInstant(ctx, prom.QueryCPUCoreUsage(orgStr, srvStr), now); err == nil {
			for _, sp := range cores {
				if core := sp.Metric["core"]; core != "" {
					out.CPU.PerCorePercent[core] = round2(sp.Value)
				}
			}
		}
		if l1, err := s.prom.QueryInstant(ctx, prom.QueryCPULoad(orgStr, srvStr, "1m"), now); err == nil && len(l1) > 0 {
			v := round2(l1[0].Value)
			out.CPU.Load1m = &v
			out.CPU.LoadSupported = true
			if out.CPU.CoreCount != nil && *out.CPU.CoreCount > 0 && v > float64(*out.CPU.CoreCount) {
				out.CPU.SaturationWarning = true
			}
		}
		if l5, err := s.prom.QueryInstant(ctx, prom.QueryCPULoad(orgStr, srvStr, "5m"), now); err == nil && len(l5) > 0 {
			v := round2(l5[0].Value)
			out.CPU.Load5m = &v
		}
		if l15, err := s.prom.QueryInstant(ctx, prom.QueryCPULoad(orgStr, srvStr, "15m"), now); err == nil && len(l15) > 0 {
			v := round2(l15[0].Value)
			out.CPU.Load15m = &v
		}
		if out.CPU.UsagePercent != nil && *out.CPU.UsagePercent >= monitoring.DefaultCPUWarning {
			out.CPU.SaturationWarning = true
		}

		// CPU 1h peak & average
		if series, err := s.prom.QueryRange(ctx, prom.QueryCPUUsage(orgStr, srvStr), now.Add(-time.Hour), now, 30*time.Second); err == nil && len(series) > 0 && len(series[0].Points) > 0 {
			peak := 0.0
			sum := 0.0
			for _, p := range series[0].Points {
				if p.Value > peak {
					peak = p.Value
				}
				sum += p.Value
			}
			pk := round2(peak)
			av := round2(sum / float64(len(series[0].Points)))
			out.CPU.PeakPercent = &pk
			out.CPU.AveragePercent = &av
		}

		// Memory breakdown
		querySingle := func(metric string) *float64 {
			if sp, err := s.prom.QueryInstant(ctx, prom.QueryMemoryMetric(metric, orgStr, srvStr), now); err == nil && len(sp) > 0 {
				v := sp[0].Value
				return &v
			}
			return nil
		}
		out.Memory.TotalBytes = querySingle("cloudpulse_memory_total_bytes")
		out.Memory.UsedBytes = querySingle("cloudpulse_memory_used_bytes")
		out.Memory.AvailableBytes = querySingle("cloudpulse_memory_available_bytes")
		out.Memory.CachedBytes = querySingle("cloudpulse_memory_cached_bytes")
		out.Memory.FreeBytes = querySingle("cloudpulse_memory_free_bytes")

		// Disks by mountpoint
		diskMap := make(map[string]*DiskMountMetric)
		ensureDisk := func(sp prom.Sample) *DiskMountMetric {
			mp := sp.Metric["mountpoint"]
			d, ok := diskMap[mp]
			if !ok {
				d = &DiskMountMetric{
					Mountpoint:  mp,
					Device:      sp.Metric["device"],
					Filesystem:  sp.Metric["filesystem"],
					CollectedAt: sp.Timestamp,
				}
				diskMap[mp] = d
			}
			return d
		}
		if sp, err := s.prom.QueryInstant(ctx, prom.QueryDiskMetric("cloudpulse_disk_total_bytes", orgStr, srvStr), now); err == nil {
			for _, item := range sp {
				ensureDisk(item).TotalBytes = item.Value
			}
		}
		if sp, err := s.prom.QueryInstant(ctx, prom.QueryDiskMetric("cloudpulse_disk_used_bytes", orgStr, srvStr), now); err == nil {
			for _, item := range sp {
				ensureDisk(item).UsedBytes = item.Value
			}
		}
		if sp, err := s.prom.QueryInstant(ctx, prom.QueryDiskMetric("cloudpulse_disk_free_bytes", orgStr, srvStr), now); err == nil {
			for _, item := range sp {
				ensureDisk(item).FreeBytes = item.Value
			}
		}
		if sp, err := s.prom.QueryInstant(ctx, prom.QueryDiskUsage(orgStr, srvStr), now); err == nil {
			for _, item := range sp {
				ensureDisk(item).UsagePercent = round2(item.Value)
			}
		}
		for _, d := range diskMap {
			out.Disks = append(out.Disks, *d)
		}
		sort.Slice(out.Disks, func(i, j int) bool { return out.Disks[i].Mountpoint < out.Disks[j].Mountpoint })

		// Network rates vs cumulative totals
		netMap := make(map[string]*NetworkInterfaceMetric)
		ensureNet := func(iface string) *NetworkInterfaceMetric {
			n, ok := netMap[iface]
			if !ok {
				n = &NetworkInterfaceMetric{Interface: iface, Status: "up"}
				netMap[iface] = n
			}
			return n
		}
		if sp, err := s.prom.QueryInstant(ctx, prom.QueryNetworkReceiveRate(orgStr, srvStr), now); err == nil {
			for _, item := range sp {
				if iface := item.Metric["interface"]; iface != "" {
					ensureNet(iface).ReceiveBytesPerSec = round2(item.Value)
				}
			}
		}
		if sp, err := s.prom.QueryInstant(ctx, prom.QueryNetworkTransmitRate(orgStr, srvStr), now); err == nil {
			for _, item := range sp {
				if iface := item.Metric["interface"]; iface != "" {
					ensureNet(iface).TransmitBytesPerSec = round2(item.Value)
				}
			}
		}
		if sp, err := s.prom.QueryInstant(ctx, prom.QueryNetworkTotal("cloudpulse_network_receive_bytes_total", orgStr, srvStr), now); err == nil {
			for _, item := range sp {
				if iface := item.Metric["interface"]; iface != "" {
					ensureNet(iface).TotalReceiveBytes = item.Value
				}
			}
		}
		if sp, err := s.prom.QueryInstant(ctx, prom.QueryNetworkTotal("cloudpulse_network_transmit_bytes_total", orgStr, srvStr), now); err == nil {
			for _, item := range sp {
				if iface := item.Metric["interface"]; iface != "" {
					ensureNet(iface).TotalTransmitBytes = item.Value
				}
			}
		}
		if sp, err := s.prom.QueryInstant(ctx, prom.QueryNetworkTotal("cloudpulse_network_receive_packets_total", orgStr, srvStr), now); err == nil {
			for _, item := range sp {
				if iface := item.Metric["interface"]; iface != "" {
					ensureNet(iface).ReceivePackets = item.Value
				}
			}
		}
		if sp, err := s.prom.QueryInstant(ctx, prom.QueryNetworkTotal("cloudpulse_network_transmit_packets_total", orgStr, srvStr), now); err == nil {
			for _, item := range sp {
				if iface := item.Metric["interface"]; iface != "" {
					ensureNet(iface).TransmitPackets = item.Value
				}
			}
		}
		if sp, err := s.prom.QueryInstant(ctx, prom.QueryNetworkTotal("cloudpulse_network_receive_errors_total", orgStr, srvStr), now); err == nil {
			for _, item := range sp {
				if iface := item.Metric["interface"]; iface != "" {
					ensureNet(iface).ReceiveErrors = item.Value
				}
			}
		}
		if sp, err := s.prom.QueryInstant(ctx, prom.QueryNetworkTotal("cloudpulse_network_transmit_errors_total", orgStr, srvStr), now); err == nil {
			for _, item := range sp {
				if iface := item.Metric["interface"]; iface != "" {
					ensureNet(iface).TransmitErrors = item.Value
				}
			}
		}
		for _, n := range netMap {
			out.Networks = append(out.Networks, *n)
		}
		sort.Slice(out.Networks, func(i, j int) bool { return out.Networks[i].Interface < out.Networks[j].Interface })
	}

	// Services on this server
	svcs, _ := s.ListServicesEnriched(ctx, orgID, &serverID)
	out.Services = svcs

	// Recent activity for this server
	logs, _, _ := s.store.ListAuditLogs(ctx, orgID, "", "", 1, 20)
	for _, l := range logs {
		if l.ResourceID == srvStr || l.ResourceType == "server" {
			out.RecentActivity = append(out.RecentActivity, InfrastructureEvent{
				ID:           l.ID.String(),
				Action:       l.Action,
				Title:        humanizeAction(l.Action, l.ResourceType),
				ResourceType: l.ResourceType,
				ResourceID:   l.ResourceID,
				Actor:        firstNonEmpty(l.ActorName, l.ActorEmail, "System"),
				Severity:     actionSeverity(l.Action),
				Timestamp:    l.CreatedAt,
			})
		}
	}

	return out, nil
}

// ServerHistoryResponse returns historical CPU, memory, disk, and network rate series for one server.
type ServerHistoryResponse struct {
	PrometheusAvailable bool              `json:"prometheus_available"`
	Range               string            `json:"range"`
	Start               time.Time         `json:"start"`
	End                 time.Time         `json:"end"`
	CPU                 []SimpleDataPoint `json:"cpu"`
	Memory              []SimpleDataPoint `json:"memory"`
	Disk                []SimpleDataPoint `json:"disk"`
	NetworkRxRate       []SimpleDataPoint `json:"network_rx_rate"`
	NetworkTxRate       []SimpleDataPoint `json:"network_tx_rate"`
}

func (s *MonitoringService) GetServerHistory(ctx context.Context, orgID, serverID uuid.UUID, rangeKey, startStr, endStr string) (ServerHistoryResponse, error) {
	if _, err := s.store.GetServer(ctx, orgID, serverID); err != nil {
		return ServerHistoryResponse{}, err
	}
	start, end, step := monitoring.ParseTimeRange(rangeKey, startStr, endStr)
	orgStr := orgID.String()
	srvStr := serverID.String()

	out := ServerHistoryResponse{
		Range:         rangeKey,
		Start:         start,
		End:           end,
		CPU:           []SimpleDataPoint{},
		Memory:        []SimpleDataPoint{},
		Disk:          []SimpleDataPoint{},
		NetworkRxRate: []SimpleDataPoint{},
		NetworkTxRate: []SimpleDataPoint{},
	}

	cpuS, err := s.prom.QueryRange(ctx, prom.QueryCPUUsage(orgStr, srvStr), start, end, step)
	if err != nil {
		out.PrometheusAvailable = false
		return out, nil
	}
	out.PrometheusAvailable = true
	out.CPU = extractFirstSeries(cpuS)

	if memS, err := s.prom.QueryRange(ctx, prom.QueryMemoryUsage(orgStr, srvStr), start, end, step); err == nil {
		out.Memory = extractFirstSeries(memS)
	}
	if diskS, err := s.prom.QueryRange(ctx, "max("+prom.QueryDiskUsage(orgStr, srvStr)+")", start, end, step); err == nil {
		out.Disk = extractFirstSeries(diskS)
	}
	if rxS, err := s.prom.QueryRange(ctx, "sum("+prom.QueryNetworkReceiveRate(orgStr, srvStr)+")", start, end, step); err == nil {
		out.NetworkRxRate = extractFirstSeries(rxS)
	}
	if txS, err := s.prom.QueryRange(ctx, "sum("+prom.QueryNetworkTransmitRate(orgStr, srvStr)+")", start, end, step); err == nil {
		out.NetworkTxRate = extractFirstSeries(txS)
	}
	return out, nil
}

// ListServicesEnriched enriches configured service checks with real Prometheus measurements.
func (s *MonitoringService) ListServicesEnriched(ctx context.Context, orgID uuid.UUID, serverID *uuid.UUID) ([]models.ServiceCheck, error) {
	checks, err := s.store.ListServiceChecks(ctx, orgID, serverID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	orgStr := orgID.String()
	srvFilter := ""
	if serverID != nil {
		srvFilter = serverID.String()
	}

	upSamples, errProm := s.prom.QueryInstant(ctx, prom.QueryServiceUp(orgStr, srvFilter), now)
	if errProm != nil {
		for i := range checks {
			checks[i].Status = models.StatusUnknown
		}
		return checks, nil
	}

	durSamples, _ := s.prom.QueryInstant(ctx, prom.QueryServiceDuration(orgStr, srvFilter), now)
	codeSamples, _ := s.prom.QueryInstant(ctx, prom.QueryServiceStatusCode(orgStr, srvFilter), now)
	tlsSamples, _ := s.prom.QueryInstant(ctx, prom.QueryServiceTLSExpiry(orgStr, srvFilter), now)

	// Query 24h range for valid-interval availability calculation (never treat missing samples as UP!)
	availSeries, _ := s.prom.QueryRange(ctx, prom.QueryServiceUp(orgStr, srvFilter), now.Add(-24*time.Hour), now, 5*time.Minute)

	keyFor := func(sid, name string) string { return sid + "::" + name }
	upMap := make(map[string]prom.Sample)
	for _, sp := range upSamples {
		upMap[keyFor(sp.Metric["cloudpulse_server_id"], sp.Metric["service"])] = sp
	}
	durMap := make(map[string]float64)
	for _, sp := range durSamples {
		durMap[keyFor(sp.Metric["cloudpulse_server_id"], sp.Metric["service"])] = round2(sp.Value * 1000)
	}
	codeMap := make(map[string]int)
	for _, sp := range codeSamples {
		codeMap[keyFor(sp.Metric["cloudpulse_server_id"], sp.Metric["service"])] = int(sp.Value)
	}
	tlsMap := make(map[string]float64)
	for _, sp := range tlsSamples {
		tlsMap[keyFor(sp.Metric["cloudpulse_server_id"], sp.Metric["service"])] = sp.Value
	}

	availPctMap := make(map[string]float64)
	availCountMap := make(map[string]int)
	for _, series := range availSeries {
		k := keyFor(series.Metric["cloudpulse_server_id"], series.Metric["service"])
		totalValid := len(series.Points)
		if totalValid == 0 {
			continue
		}
		successCount := 0
		for _, p := range series.Points {
			if p.Value >= 1 {
				successCount++
			}
		}
		availCountMap[k] = totalValid
		availPctMap[k] = round2(float64(successCount) / float64(totalValid) * 100)
	}

	for i := range checks {
		k := keyFor(checks[i].ServerID.String(), checks[i].Name)
		if !checks[i].Enabled {
			checks[i].Status = models.StatusUnknown
			continue
		}
		if sp, ok := upMap[k]; ok {
			t := sp.Timestamp
			checks[i].LastCheckedAt = &t
			if now.Sub(t) > monitoring.FreshnessThreshold*2 {
				checks[i].Status = models.StatusUnknown
			} else if sp.Value >= 1 {
				checks[i].Status = models.StatusUp
			} else {
				checks[i].Status = models.StatusDown
			}
		} else {
			checks[i].Status = models.StatusUnknown
		}
		if ms, ok := durMap[k]; ok {
			v := ms
			checks[i].LatencyMs = &v
		}
		if code, ok := codeMap[k]; ok {
			c := code
			checks[i].HTTPStatus = &c
		}
		if tlsSec, ok := tlsMap[k]; ok {
			v := tlsSec
			checks[i].TLSExpirySecondsLeft = &v
		}
		if cnt, ok := availCountMap[k]; ok && cnt > 0 {
			pct := availPctMap[k]
			checks[i].Availability24h = &pct
			checks[i].ValidIntervals24h = cnt
		}
	}

	return checks, nil
}

// ServiceHistoryResponse returns availability and response time history for one service check.
type ServiceHistoryResponse struct {
	PrometheusAvailable bool                `json:"prometheus_available"`
	Service             models.ServiceCheck `json:"service"`
	AvailabilityPercent *float64            `json:"availability_percent"`
	ValidIntervals      int                 `json:"valid_intervals"`
	FailedIntervals     int                 `json:"failed_intervals"`
	AverageLatencyMs    *float64            `json:"average_latency_ms"`
	UpTimeline          []SimpleDataPoint   `json:"up_timeline"`
	LatencyTimeline     []SimpleDataPoint   `json:"latency_timeline"`
}

func (s *MonitoringService) GetServiceHistory(ctx context.Context, orgID, serviceID uuid.UUID, rangeKey string) (ServiceHistoryResponse, error) {
	sc, err := s.store.GetServiceCheck(ctx, orgID, serviceID)
	if err != nil {
		return ServiceHistoryResponse{}, err
	}
	start, end, step := monitoring.ParseTimeRange(rangeKey, "", "")
	sel := prom.CombineSelectors(
		prom.OrgFilter(orgID.String()),
		prom.ServerFilter(sc.ServerID.String()),
		fmt.Sprintf(`service="%s"`, prom.EscapeLabelValue(sc.Name)),
	)

	out := ServiceHistoryResponse{
		Service:         sc,
		UpTimeline:      []SimpleDataPoint{},
		LatencyTimeline: []SimpleDataPoint{},
	}

	upSeries, err := s.prom.QueryRange(ctx, "cloudpulse_service_up"+sel, start, end, step)
	if err != nil {
		out.PrometheusAvailable = false
		return out, nil
	}
	out.PrometheusAvailable = true
	out.UpTimeline = extractFirstSeries(upSeries)

	if len(out.UpTimeline) > 0 {
		okCnt := 0
		for _, p := range out.UpTimeline {
			if p.Value >= 1 {
				okCnt++
			}
		}
		out.ValidIntervals = len(out.UpTimeline)
		out.FailedIntervals = out.ValidIntervals - okCnt
		pct := round2(float64(okCnt) / float64(out.ValidIntervals) * 100)
		out.AvailabilityPercent = &pct
	}

	if latSeries, err := s.prom.QueryRange(ctx, "cloudpulse_service_response_duration_seconds"+sel+" * 1000", start, end, step); err == nil {
		out.LatencyTimeline = extractFirstSeries(latSeries)
		if len(out.LatencyTimeline) > 0 {
			sum := 0.0
			for _, p := range out.LatencyTimeline {
				sum += p.Value
			}
			avg := round2(sum / float64(len(out.LatencyTimeline)))
			out.AverageLatencyMs = &avg
		}
	}

	return out, nil
}

// Helper utilities
func indexSamplesByServer(samples []prom.Sample) map[string]prom.Sample {
	m := make(map[string]prom.Sample, len(samples))
	for _, sp := range samples {
		if sid := sp.Metric["cloudpulse_server_id"]; sid != "" {
			m[sid] = sp
		}
	}
	return m
}

func sumSamples(samples []prom.Sample) float64 {
	sum := 0.0
	for _, sp := range samples {
		if !math.IsNaN(sp.Value) && !math.IsInf(sp.Value, 0) {
			sum += sp.Value
		}
	}
	return sum
}

func extractFirstSeries(series []prom.MatrixSeries) []SimpleDataPoint {
	if len(series) == 0 {
		return []SimpleDataPoint{}
	}
	pts := make([]SimpleDataPoint, 0, len(series[0].Points))
	for _, p := range series[0].Points {
		pts = append(pts, SimpleDataPoint{Timestamp: p.Timestamp, Value: round2(p.Value)})
	}
	return pts
}

func round2(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

func computeFingerprint(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(labels[k])
		b.WriteString(";")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:8])
}

func humanizeAction(action, resourceType string) string {
	a := strings.ReplaceAll(action, ".", " ")
	a = strings.ReplaceAll(a, "_", " ")
	if len(a) == 0 {
		return resourceType + " updated"
	}
	return strings.ToUpper(a[:1]) + a[1:]
}

func actionSeverity(action string) string {
	l := strings.ToLower(action)
	switch {
	case strings.Contains(l, "fail") || strings.Contains(l, "delete") || strings.Contains(l, "down"):
		return "warning"
	case strings.Contains(l, "critical"):
		return "critical"
	default:
		return "info"
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
