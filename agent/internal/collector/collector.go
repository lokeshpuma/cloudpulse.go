package collector

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"

	"github.com/cloudpulse/cloudpulse-go/agent/internal/config"
)

// CollectorStatus reports the latest collection state for health diagnostics.
type CollectorStatus struct {
	Name        string    `json:"name"`
	Enabled     bool      `json:"enabled"`
	Status      string    `json:"status"` // ok, error, unsupported, disabled
	LastError   string    `json:"last_error,omitempty"`
	LastSuccess time.Time `json:"last_success,omitempty"`
}

// Snapshot holds overall collector health metadata.
type Snapshot struct {
	Healthy        bool                       `json:"healthy"`
	LastCollection time.Time                  `json:"last_collection,omitempty"`
	Collectors     map[string]CollectorStatus `json:"collectors"`
}

// SystemProvider abstracts gopsutil calls for deterministic unit testing.
type SystemProvider interface {
	CPUPercent(ctx context.Context, perCPU bool) ([]float64, error)
	CPUCounts(ctx context.Context) (int, error)
	LoadAvg(ctx context.Context) (*load.AvgStat, error)
	VirtualMemory(ctx context.Context) (*mem.VirtualMemoryStat, error)
	DiskPartitions(ctx context.Context) ([]disk.PartitionStat, error)
	DiskUsage(ctx context.Context, path string) (*disk.UsageStat, error)
	NetIOCounters(ctx context.Context) ([]gnet.IOCountersStat, error)
}

type gopsutilProvider struct{}

func (gopsutilProvider) CPUPercent(ctx context.Context, perCPU bool) ([]float64, error) {
	return cpu.PercentWithContext(ctx, 250*time.Millisecond, perCPU)
}

func (gopsutilProvider) CPUCounts(ctx context.Context) (int, error) {
	return cpu.CountsWithContext(ctx, true)
}

func (gopsutilProvider) LoadAvg(ctx context.Context) (*load.AvgStat, error) {
	return load.AvgWithContext(ctx)
}

func (gopsutilProvider) VirtualMemory(ctx context.Context) (*mem.VirtualMemoryStat, error) {
	return mem.VirtualMemoryWithContext(ctx)
}

func (gopsutilProvider) DiskPartitions(ctx context.Context) ([]disk.PartitionStat, error) {
	return disk.PartitionsWithContext(ctx, false)
}

func (gopsutilProvider) DiskUsage(ctx context.Context, path string) (*disk.UsageStat, error) {
	return disk.UsageWithContext(ctx, path)
}

func (gopsutilProvider) NetIOCounters(ctx context.Context) ([]gnet.IOCountersStat, error) {
	return gnet.IOCountersWithContext(ctx, true)
}

// Manager coordinates all host metric collectors and exposes them via a Prometheus registry.
type Manager struct {
	cfg      config.Config
	provider SystemProvider
	logger   *slog.Logger
	registry *prometheus.Registry
	version  string

	// CPU metrics
	cpuUsagePercent     prometheus.Gauge
	cpuCoreUsagePercent *prometheus.GaugeVec
	cpuCoreCount        prometheus.Gauge
	cpuLoad1m           prometheus.Gauge
	cpuLoad5m           prometheus.Gauge
	cpuLoad15m          prometheus.Gauge
	cpuLoadRegistered   bool

	// Memory metrics
	memTotalBytes     prometheus.Gauge
	memUsedBytes      prometheus.Gauge
	memAvailableBytes prometheus.Gauge
	memUsagePercent   prometheus.Gauge
	memCachedBytes    prometheus.Gauge
	memFreeBytes      prometheus.Gauge

	// Disk metrics
	diskTotalBytes   *prometheus.GaugeVec
	diskUsedBytes    *prometheus.GaugeVec
	diskFreeBytes    *prometheus.GaugeVec
	diskUsagePercent *prometheus.GaugeVec

	// Network metrics (cumulative counters backed by OS monotonic counters)
	netReceiveBytes    *prometheus.GaugeVec
	netTransmitBytes   *prometheus.GaugeVec
	netReceivePackets  *prometheus.GaugeVec
	netTransmitPackets *prometheus.GaugeVec
	netReceiveErrors   *prometheus.GaugeVec
	netTransmitErrors  *prometheus.GaugeVec

	// Agent metrics
	agentUp               prometheus.Gauge
	agentBuildInfo        *prometheus.GaugeVec
	agentCollectDuration  prometheus.Gauge
	agentCollectErrors    *prometheus.CounterVec
	agentLastCollectStamp prometheus.Gauge

	mu             sync.RWMutex
	lastCollection time.Time
	statuses       map[string]CollectorStatus
}

// NewManager creates a Manager with a dedicated Prometheus registry.
func NewManager(cfg config.Config, version string, logger *slog.Logger, provider SystemProvider) *Manager {
	if provider == nil {
		provider = gopsutilProvider{}
	}
	if logger == nil {
		logger = slog.Default()
	}

	reg := prometheus.NewRegistry()
	constLabels := prometheus.Labels{
		"server_id":   cfg.Agent.ID,
		"server_name": cfg.Agent.Name,
		"environment": cfg.Agent.Environment,
	}

	if cfg.Collection.IncludeGoMetric {
		reg.MustRegister(collectors.NewGoCollector())
		reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	}

	m := &Manager{
		cfg:      cfg,
		provider: provider,
		logger:   logger,
		registry: reg,
		version:  version,
		statuses: make(map[string]CollectorStatus),

		cpuUsagePercent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cloudpulse_cpu_usage_percent",
			Help:        "Aggregate host CPU utilization percentage (0-100).",
			ConstLabels: constLabels,
		}),
		cpuCoreUsagePercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_cpu_core_usage_percent",
			Help:        "Per-core CPU utilization percentage (0-100).",
			ConstLabels: constLabels,
		}, []string{"core"}),
		cpuCoreCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cloudpulse_cpu_core_count",
			Help:        "Total logical CPU cores reported by the host.",
			ConstLabels: constLabels,
		}),
		cpuLoad1m: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cloudpulse_cpu_load_1m",
			Help:        "Host 1-minute load average where supported by the OS.",
			ConstLabels: constLabels,
		}),
		cpuLoad5m: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cloudpulse_cpu_load_5m",
			Help:        "Host 5-minute load average where supported by the OS.",
			ConstLabels: constLabels,
		}),
		cpuLoad15m: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cloudpulse_cpu_load_15m",
			Help:        "Host 15-minute load average where supported by the OS.",
			ConstLabels: constLabels,
		}),

		memTotalBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cloudpulse_memory_total_bytes",
			Help:        "Total physical host memory in bytes.",
			ConstLabels: constLabels,
		}),
		memUsedBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cloudpulse_memory_used_bytes",
			Help:        "Used host memory in bytes (excluding reclaimable cache/buffers per OS definition).",
			ConstLabels: constLabels,
		}),
		memAvailableBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cloudpulse_memory_available_bytes",
			Help:        "OS-reported memory available for starting new applications without swapping, in bytes.",
			ConstLabels: constLabels,
		}),
		memUsagePercent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cloudpulse_memory_usage_percent",
			Help:        "Host memory utilization percentage calculated from OS-reported used/available memory.",
			ConstLabels: constLabels,
		}),
		memCachedBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cloudpulse_memory_cached_bytes",
			Help:        "Host page cache and buffer memory in bytes where reported by the OS.",
			ConstLabels: constLabels,
		}),
		memFreeBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cloudpulse_memory_free_bytes",
			Help:        "Unused physical host memory in bytes.",
			ConstLabels: constLabels,
		}),

		diskTotalBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_disk_total_bytes",
			Help:        "Total disk capacity in bytes for a monitored mount point.",
			ConstLabels: constLabels,
		}, []string{"mountpoint", "device", "filesystem"}),
		diskUsedBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_disk_used_bytes",
			Help:        "Used disk space in bytes for a monitored mount point.",
			ConstLabels: constLabels,
		}, []string{"mountpoint", "device", "filesystem"}),
		diskFreeBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_disk_free_bytes",
			Help:        "Available disk space in bytes for a monitored mount point.",
			ConstLabels: constLabels,
		}, []string{"mountpoint", "device", "filesystem"}),
		diskUsagePercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_disk_usage_percent",
			Help:        "Disk utilization percentage for a monitored mount point.",
			ConstLabels: constLabels,
		}, []string{"mountpoint", "device", "filesystem"}),

		netReceiveBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_network_receive_bytes_total",
			Help:        "Cumulative network bytes received on the interface (use PromQL rate() for throughput).",
			ConstLabels: constLabels,
		}, []string{"interface"}),
		netTransmitBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_network_transmit_bytes_total",
			Help:        "Cumulative network bytes transmitted on the interface (use PromQL rate() for throughput).",
			ConstLabels: constLabels,
		}, []string{"interface"}),
		netReceivePackets: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_network_receive_packets_total",
			Help:        "Cumulative network packets received on the interface.",
			ConstLabels: constLabels,
		}, []string{"interface"}),
		netTransmitPackets: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_network_transmit_packets_total",
			Help:        "Cumulative network packets transmitted on the interface.",
			ConstLabels: constLabels,
		}, []string{"interface"}),
		netReceiveErrors: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_network_receive_errors_total",
			Help:        "Cumulative network receive errors on the interface.",
			ConstLabels: constLabels,
		}, []string{"interface"}),
		netTransmitErrors: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_network_transmit_errors_total",
			Help:        "Cumulative network transmit errors on the interface.",
			ConstLabels: constLabels,
		}, []string{"interface"}),

		agentUp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cloudpulse_agent_up",
			Help:        "1 if the CloudPulse agent is running and healthy, 0 otherwise.",
			ConstLabels: constLabels,
		}),
		agentBuildInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_agent_build_info",
			Help:        "Build and host metadata for the CloudPulse agent.",
			ConstLabels: constLabels,
		}, []string{"version", "os", "arch", "hostname"}),
		agentCollectDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cloudpulse_agent_collection_duration_seconds",
			Help:        "Duration of the most recent metric collection cycle in seconds.",
			ConstLabels: constLabels,
		}),
		agentCollectErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "cloudpulse_agent_collection_errors_total",
			Help:        "Total metric collection errors grouped by collector name.",
			ConstLabels: constLabels,
		}, []string{"collector"}),
		agentLastCollectStamp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "cloudpulse_agent_last_collection_timestamp_seconds",
			Help:        "Unix timestamp of the last completed metric collection cycle.",
			ConstLabels: constLabels,
		}),
	}

	reg.MustRegister(
		m.agentUp,
		m.agentBuildInfo,
		m.agentCollectDuration,
		m.agentCollectErrors,
		m.agentLastCollectStamp,
	)

	if cfg.CPU.Enabled {
		reg.MustRegister(m.cpuUsagePercent, m.cpuCoreCount)
		if cfg.CPU.PerCoreEnabled {
			reg.MustRegister(m.cpuCoreUsagePercent)
		}
	}
	if cfg.Memory.Enabled {
		reg.MustRegister(
			m.memTotalBytes,
			m.memUsedBytes,
			m.memAvailableBytes,
			m.memUsagePercent,
			m.memCachedBytes,
			m.memFreeBytes,
		)
	}
	if cfg.Disk.Enabled {
		reg.MustRegister(m.diskTotalBytes, m.diskUsedBytes, m.diskFreeBytes, m.diskUsagePercent)
	}
	if cfg.Network.Enabled {
		reg.MustRegister(
			m.netReceiveBytes,
			m.netTransmitBytes,
			m.netReceivePackets,
			m.netTransmitPackets,
			m.netReceiveErrors,
			m.netTransmitErrors,
		)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = cfg.Agent.Name
	}
	m.agentUp.Set(1)
	m.agentBuildInfo.WithLabelValues(version, runtime.GOOS, runtime.GOARCH, hostname).Set(1)

	return m
}

// Registry returns the Prometheus registry managed by this agent.
func (m *Manager) Registry() *prometheus.Registry {
	return m.registry
}

// CollectOnce runs one full collection cycle with per-collector isolation.
func (m *Manager) CollectOnce(ctx context.Context) {
	start := time.Now()
	cctx, cancel := context.WithTimeout(ctx, m.cfg.Collection.Timeout.Duration)
	defer cancel()

	if m.cfg.CPU.Enabled {
		m.collectCPU(cctx)
	} else {
		m.setStatus("cpu", CollectorStatus{Name: "cpu", Enabled: false, Status: "disabled"})
	}

	if m.cfg.Memory.Enabled {
		m.collectMemory(cctx)
	} else {
		m.setStatus("memory", CollectorStatus{Name: "memory", Enabled: false, Status: "disabled"})
	}

	if m.cfg.Disk.Enabled {
		m.collectDisk(cctx)
	} else {
		m.setStatus("disk", CollectorStatus{Name: "disk", Enabled: false, Status: "disabled"})
	}

	if m.cfg.Network.Enabled {
		m.collectNetwork(cctx)
	} else {
		m.setStatus("network", CollectorStatus{Name: "network", Enabled: false, Status: "disabled"})
	}

	elapsed := time.Since(start)
	now := time.Now().UTC()
	m.agentCollectDuration.Set(elapsed.Seconds())
	m.agentLastCollectStamp.Set(float64(now.Unix()))

	m.mu.Lock()
	m.lastCollection = now
	m.mu.Unlock()
}

// Run starts the periodic collection loop until ctx is canceled.
func (m *Manager) Run(ctx context.Context) {
	m.CollectOnce(ctx)
	ticker := time.NewTicker(m.cfg.Collection.Interval.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.CollectOnce(ctx)
		}
	}
}

// Health returns the current collector health snapshot.
func (m *Manager) Health() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	copyMap := make(map[string]CollectorStatus, len(m.statuses))
	healthy := true
	enabledCount := 0
	okCount := 0

	for k, st := range m.statuses {
		copyMap[k] = st
		if st.Enabled {
			enabledCount++
			if st.Status == "ok" || st.Status == "unsupported" {
				okCount++
			}
		}
	}

	if enabledCount > 0 && okCount == 0 {
		healthy = false
	}

	return Snapshot{
		Healthy:        healthy,
		LastCollection: m.lastCollection,
		Collectors:     copyMap,
	}
}

func (m *Manager) setStatus(name string, st CollectorStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses[name] = st
}

func (m *Manager) collectCPU(ctx context.Context) {
	var errs []string

	if totals, err := m.provider.CPUPercent(ctx, false); err != nil {
		errs = append(errs, fmt.Sprintf("cpu usage: %v", err))
	} else if len(totals) > 0 && !math.IsNaN(totals[0]) {
		m.cpuUsagePercent.Set(clampPercent(totals[0]))
	}

	if m.cfg.CPU.PerCoreEnabled {
		if perCore, err := m.provider.CPUPercent(ctx, true); err == nil {
			m.cpuCoreUsagePercent.Reset()
			for idx, pct := range perCore {
				if !math.IsNaN(pct) {
					m.cpuCoreUsagePercent.WithLabelValues(strconv.Itoa(idx)).Set(clampPercent(pct))
				}
			}
		}
	}

	if cores, err := m.provider.CPUCounts(ctx); err != nil {
		errs = append(errs, fmt.Sprintf("cpu count: %v", err))
	} else if cores > 0 {
		m.cpuCoreCount.Set(float64(cores))
	}

	if avg, err := m.provider.LoadAvg(ctx); err == nil && avg != nil {
		m.registerLoadMetricsOnce()
		m.cpuLoad1m.Set(avg.Load1)
		m.cpuLoad5m.Set(avg.Load5)
		m.cpuLoad15m.Set(avg.Load15)
	}

	if len(errs) > 0 {
		msg := strings.Join(errs, "; ")
		m.agentCollectErrors.WithLabelValues("cpu").Inc()
		m.logger.Warn("cpu collection failed", "error", msg)
		m.setStatus("cpu", CollectorStatus{Name: "cpu", Enabled: true, Status: "error", LastError: msg})
		return
	}
	m.setStatus("cpu", CollectorStatus{Name: "cpu", Enabled: true, Status: "ok", LastSuccess: time.Now().UTC()})
}

func (m *Manager) registerLoadMetricsOnce() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cpuLoadRegistered {
		return
	}
	_ = m.registry.Register(m.cpuLoad1m)
	_ = m.registry.Register(m.cpuLoad5m)
	_ = m.registry.Register(m.cpuLoad15m)
	m.cpuLoadRegistered = true
}

func (m *Manager) collectMemory(ctx context.Context) {
	vm, err := m.provider.VirtualMemory(ctx)
	if err != nil {
		m.agentCollectErrors.WithLabelValues("memory").Inc()
		m.logger.Warn("memory collection failed", "error", err)
		m.setStatus("memory", CollectorStatus{Name: "memory", Enabled: true, Status: "error", LastError: err.Error()})
		return
	}

	m.memTotalBytes.Set(float64(vm.Total))
	m.memUsedBytes.Set(float64(vm.Used))
	// Use OS-reported Available directly rather than Total - Free.
	m.memAvailableBytes.Set(float64(vm.Available))
	m.memUsagePercent.Set(clampPercent(vm.UsedPercent))
	m.memCachedBytes.Set(float64(vm.Cached + vm.Buffers))
	m.memFreeBytes.Set(float64(vm.Free))

	m.setStatus("memory", CollectorStatus{Name: "memory", Enabled: true, Status: "ok", LastSuccess: time.Now().UTC()})
}

func (m *Manager) collectDisk(ctx context.Context) {
	parts, err := m.provider.DiskPartitions(ctx)
	if err != nil {
		m.agentCollectErrors.WithLabelValues("disk").Inc()
		m.logger.Warn("disk partition discovery failed", "error", err)
		m.setStatus("disk", CollectorStatus{Name: "disk", Enabled: true, Status: "error", LastError: err.Error()})
		return
	}

	m.diskTotalBytes.Reset()
	m.diskUsedBytes.Reset()
	m.diskFreeBytes.Reset()
	m.diskUsagePercent.Reset()

	seenMounts := make(map[string]struct{})
	collected := 0
	var lastErr error

	for _, part := range parts {
		if m.isExcludedMount(part.Mountpoint) || (part.Mountpoint != "/" && m.isExcludedFS(part.Fstype)) {
			continue
		}
		if _, dup := seenMounts[part.Mountpoint]; dup {
			continue
		}
		seenMounts[part.Mountpoint] = struct{}{}

		usage, err := m.provider.DiskUsage(ctx, part.Mountpoint)
		if err != nil {
			lastErr = err
			continue
		}
		if usage.Total == 0 {
			continue
		}

		fsType := part.Fstype
		if fsType == "" {
			fsType = usage.Fstype
		}
		m.diskTotalBytes.WithLabelValues(part.Mountpoint, part.Device, fsType).Set(float64(usage.Total))
		m.diskUsedBytes.WithLabelValues(part.Mountpoint, part.Device, fsType).Set(float64(usage.Used))
		m.diskFreeBytes.WithLabelValues(part.Mountpoint, part.Device, fsType).Set(float64(usage.Free))
		m.diskUsagePercent.WithLabelValues(part.Mountpoint, part.Device, fsType).Set(clampPercent(usage.UsedPercent))
		collected++
	}

	// In minimal containers where Partitions(all=false) omits the overlay root mount, collect "/" directly.
	if collected == 0 && !m.isExcludedMount("/") {
		if usage, err := m.provider.DiskUsage(ctx, "/"); err == nil && usage.Total > 0 {
			fsType := usage.Fstype
			if fsType == "" {
				fsType = "rootfs"
			}
			m.diskTotalBytes.WithLabelValues("/", "rootfs", fsType).Set(float64(usage.Total))
			m.diskUsedBytes.WithLabelValues("/", "rootfs", fsType).Set(float64(usage.Used))
			m.diskFreeBytes.WithLabelValues("/", "rootfs", fsType).Set(float64(usage.Free))
			m.diskUsagePercent.WithLabelValues("/", "rootfs", fsType).Set(clampPercent(usage.UsedPercent))
			collected++
		}
	}

	if collected == 0 && lastErr != nil {
		m.agentCollectErrors.WithLabelValues("disk").Inc()
		m.logger.Warn("disk usage collection failed", "error", lastErr)
		m.setStatus("disk", CollectorStatus{Name: "disk", Enabled: true, Status: "error", LastError: lastErr.Error()})
		return
	}

	m.setStatus("disk", CollectorStatus{Name: "disk", Enabled: true, Status: "ok", LastSuccess: time.Now().UTC()})
}

func (m *Manager) collectNetwork(ctx context.Context) {
	counters, err := m.provider.NetIOCounters(ctx)
	if err != nil {
		m.agentCollectErrors.WithLabelValues("network").Inc()
		m.logger.Warn("network collection failed", "error", err)
		m.setStatus("network", CollectorStatus{Name: "network", Enabled: true, Status: "error", LastError: err.Error()})
		return
	}

	m.netReceiveBytes.Reset()
	m.netTransmitBytes.Reset()
	m.netReceivePackets.Reset()
	m.netTransmitPackets.Reset()
	m.netReceiveErrors.Reset()
	m.netTransmitErrors.Reset()

	for _, nic := range counters {
		if m.isExcludedInterface(nic.Name) {
			continue
		}
		m.netReceiveBytes.WithLabelValues(nic.Name).Set(float64(nic.BytesRecv))
		m.netTransmitBytes.WithLabelValues(nic.Name).Set(float64(nic.BytesSent))
		m.netReceivePackets.WithLabelValues(nic.Name).Set(float64(nic.PacketsRecv))
		m.netTransmitPackets.WithLabelValues(nic.Name).Set(float64(nic.PacketsSent))
		m.netReceiveErrors.WithLabelValues(nic.Name).Set(float64(nic.Errin))
		m.netTransmitErrors.WithLabelValues(nic.Name).Set(float64(nic.Errout))
	}

	m.setStatus("network", CollectorStatus{Name: "network", Enabled: true, Status: "ok", LastSuccess: time.Now().UTC()})
}

func (m *Manager) isExcludedMount(mp string) bool {
	for _, ex := range m.cfg.Disk.ExcludedMountpoints {
		ex = strings.TrimSpace(ex)
		if ex == "" {
			continue
		}
		if mp == ex || strings.HasPrefix(mp, ex+"/") {
			return true
		}
	}
	return false
}

func (m *Manager) isExcludedFS(fs string) bool {
	fsLower := strings.ToLower(strings.TrimSpace(fs))
	for _, ex := range m.cfg.Disk.ExcludedFilesystems {
		if strings.ToLower(strings.TrimSpace(ex)) == fsLower {
			return true
		}
	}
	return false
}

func (m *Manager) isExcludedInterface(iface string) bool {
	for _, ex := range m.cfg.Network.ExcludedInterfaces {
		if strings.TrimSpace(ex) == iface {
			return true
		}
	}
	return false
}

func clampPercent(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return math.Round(v*100) / 100
}
