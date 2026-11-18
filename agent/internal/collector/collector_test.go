package collector

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudpulse/cloudpulse-go/agent/internal/config"
)

type fakeSystemProvider struct {
	cpuTotal []float64
	cpuCores []float64
	cpuCount int
	cpuErr   error
	loadAvg  *load.AvgStat
	loadErr  error
	vm       *mem.VirtualMemoryStat
	vmErr    error
	parts    []disk.PartitionStat
	partsErr error
	usages   map[string]*disk.UsageStat
	nets     []gnet.IOCountersStat
	netErr   error
}

func (f fakeSystemProvider) CPUPercent(_ context.Context, perCPU bool) ([]float64, error) {
	if f.cpuErr != nil {
		return nil, f.cpuErr
	}
	if perCPU {
		return f.cpuCores, nil
	}
	return f.cpuTotal, nil
}

func (f fakeSystemProvider) CPUCounts(_ context.Context) (int, error) {
	if f.cpuErr != nil {
		return 0, f.cpuErr
	}
	return f.cpuCount, nil
}

func (f fakeSystemProvider) LoadAvg(_ context.Context) (*load.AvgStat, error) {
	return f.loadAvg, f.loadErr
}

func (f fakeSystemProvider) VirtualMemory(_ context.Context) (*mem.VirtualMemoryStat, error) {
	return f.vm, f.vmErr
}

func (f fakeSystemProvider) DiskPartitions(_ context.Context) ([]disk.PartitionStat, error) {
	return f.parts, f.partsErr
}

func (f fakeSystemProvider) DiskUsage(_ context.Context, path string) (*disk.UsageStat, error) {
	if u, ok := f.usages[path]; ok {
		return u, nil
	}
	return nil, errors.New("not found")
}

func (f fakeSystemProvider) NetIOCounters(_ context.Context) ([]gnet.IOCountersStat, error) {
	return f.nets, f.netErr
}

func TestCollectAllMetrics(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Collection.IncludeGoMetric = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	prov := fakeSystemProvider{
		cpuTotal: []float64{42.5},
		cpuCores: []float64{30.0, 55.0},
		cpuCount: 2,
		loadAvg:  &load.AvgStat{Load1: 1.2, Load5: 0.9, Load15: 0.7},
		vm: &mem.VirtualMemoryStat{
			Total:       16_000_000_000,
			Used:        6_000_000_000,
			Available:   10_000_000_000, // Note: != Free because of cache/buffers
			Free:        4_000_000_000,
			Cached:      5_000_000_000,
			Buffers:     1_000_000_000,
			UsedPercent: 37.5,
		},
		parts: []disk.PartitionStat{
			{Device: "/dev/sda1", Mountpoint: "/", Fstype: "ext4"},
			{Device: "proc", Mountpoint: "/proc", Fstype: "proc"},
			{Device: "tmpfs", Mountpoint: "/run", Fstype: "tmpfs"},
		},
		usages: map[string]*disk.UsageStat{
			"/": {Total: 100_000_000_000, Used: 45_000_000_000, Free: 55_000_000_000, UsedPercent: 45.0, Fstype: "ext4"},
		},
		nets: []gnet.IOCountersStat{
			{Name: "eth0", BytesRecv: 102400, BytesSent: 51200, PacketsRecv: 200, PacketsSent: 100, Errin: 1, Errout: 2},
			{Name: "lo", BytesRecv: 9999, BytesSent: 9999},
		},
	}

	mgr := NewManager(cfg, "1.0.0-test", logger, prov)
	mgr.CollectOnce(context.Background())

	assert.Equal(t, 42.5, testutil.ToFloat64(mgr.cpuUsagePercent))
	assert.Equal(t, float64(2), testutil.ToFloat64(mgr.cpuCoreCount))
	assert.Equal(t, 1.2, testutil.ToFloat64(mgr.cpuLoad1m))
	assert.Equal(t, float64(10_000_000_000), testutil.ToFloat64(mgr.memAvailableBytes))
	assert.Equal(t, float64(6_000_000_000), testutil.ToFloat64(mgr.memCachedBytes))
	assert.Equal(t, 45.0, testutil.ToFloat64(mgr.diskUsagePercent.WithLabelValues("/", "/dev/sda1", "ext4")))
	assert.Equal(t, float64(102400), testutil.ToFloat64(mgr.netReceiveBytes.WithLabelValues("eth0")))

	// Ensure /proc and lo were excluded
	mfs, err := mgr.Registry().Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() == "cloudpulse_disk_usage_percent" {
			assert.Len(t, mf.GetMetric(), 1)
		}
		if mf.GetName() == "cloudpulse_network_receive_bytes_total" {
			assert.Len(t, mf.GetMetric(), 1)
		}
	}

	health := mgr.Health()
	assert.True(t, health.Healthy)
	assert.Equal(t, "ok", health.Collectors["cpu"].Status)
	assert.Equal(t, "ok", health.Collectors["memory"].Status)
	assert.Equal(t, "ok", health.Collectors["disk"].Status)
	assert.Equal(t, "ok", health.Collectors["network"].Status)
}

func TestSingleCollectorFailureDoesNotCrashAgent(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Collection.IncludeGoMetric = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	prov := fakeSystemProvider{
		cpuErr: errors.New("cpu read error"),
		vm: &mem.VirtualMemoryStat{
			Total:       8_000_000_000,
			Used:        4_000_000_000,
			Available:   4_000_000_000,
			UsedPercent: 50.0,
		},
		parts: []disk.PartitionStat{},
		nets:  []gnet.IOCountersStat{},
	}

	mgr := NewManager(cfg, "1.0.0-test", logger, prov)
	mgr.CollectOnce(context.Background())

	health := mgr.Health()
	assert.True(t, health.Healthy, "agent stays healthy when other collectors succeed")
	assert.Equal(t, "error", health.Collectors["cpu"].Status)
	assert.Equal(t, "ok", health.Collectors["memory"].Status)
	assert.Equal(t, float64(1), testutil.ToFloat64(mgr.agentCollectErrors.WithLabelValues("cpu")))
}

func TestAllCollectorsFailReportsDegraded(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Collection.IncludeGoMetric = false
	cfg.Disk.Enabled = false
	cfg.Network.Enabled = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	prov := fakeSystemProvider{
		cpuErr: errors.New("boom"),
		vmErr:  errors.New("boom"),
	}

	mgr := NewManager(cfg, "1.0.0-test", logger, prov)
	mgr.CollectOnce(context.Background())

	health := mgr.Health()
	assert.False(t, health.Healthy)
}
