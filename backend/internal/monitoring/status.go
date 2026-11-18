package monitoring

import (
	"math"
	"time"

	"github.com/cloudpulse/cloudpulse-go/backend/internal/models"
)

// FreshnessThreshold defines how old a metric sample can be before it is marked stale.
const FreshnessThreshold = 90 * time.Second

// Thresholds for derived status when live alert state is absent.
const (
	DefaultCPUWarning  = 75.0
	DefaultCPUCritical = 90.0
	DefaultMemWarning  = 80.0
	DefaultMemCritical = 95.0
	DefaultDiskWarning = 80.0
	DefaultDiskCrit    = 90.0
)

// ServerLiveInput captures live Prometheus observations for a registered server.
type ServerLiveInput struct {
	MonitoringEnabled bool
	LastSeenAt        *time.Time
	TargetHealth      string // "up", "down", "unknown", ""
	UpSample          *float64
	LastSampleAt      *time.Time
	CPU               *float64
	Memory            *float64
	DiskMax           *float64
	HasCriticalAlert  bool
	HasWarningAlert   bool
	PrometheusOnline  bool
	Now               time.Time
}

// DerivedStatus represents the consistent health derivation required by PRD Section 23.
type DerivedStatus struct {
	Status       string // UP, DOWN, WARNING, CRITICAL, UNKNOWN, PENDING
	Availability string // UP, DOWN, UNKNOWN
	IsStale      bool
	DataSource   string // live_prometheus, unavailable
}

// DeriveServerStatus enforces:
// - Never convert UNKNOWN into UP
// - Newly registered server without a scrape is PENDING
// - Stale data (> freshness threshold) degrades to UNKNOWN with IsStale=true
// - Prometheus outage yields UNKNOWN with DataSource="unavailable"
func DeriveServerStatus(in ServerLiveInput) DerivedStatus {
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	}

	if !in.PrometheusOnline {
		return DerivedStatus{
			Status:       models.StatusUnknown,
			Availability: models.StatusUnknown,
			IsStale:      true,
			DataSource:   "unavailable",
		}
	}

	if !in.MonitoringEnabled {
		return DerivedStatus{
			Status:       models.StatusUnknown,
			Availability: models.StatusUnknown,
			IsStale:      false,
			DataSource:   "live_prometheus",
		}
	}

	// Never scraped yet and no sample
	if in.UpSample == nil && in.TargetHealth == "" && in.LastSeenAt == nil {
		return DerivedStatus{
			Status:       models.StatusPending,
			Availability: models.StatusUnknown,
			IsStale:      false,
			DataSource:   "live_prometheus",
		}
	}

	isStale := false
	if in.LastSampleAt != nil && in.Now.Sub(*in.LastSampleAt) > FreshnessThreshold {
		isStale = true
	}

	// Explicitly confirmed down by Prometheus up==0 or target health == down
	if (in.UpSample != nil && *in.UpSample == 0) || in.TargetHealth == "down" {
		return DerivedStatus{
			Status:       models.StatusDown,
			Availability: models.StatusDown,
			IsStale:      isStale,
			DataSource:   "live_prometheus",
		}
	}

	// Missing or stale sample -> UNKNOWN (never UP!)
	if in.UpSample == nil || isStale {
		return DerivedStatus{
			Status:       models.StatusUnknown,
			Availability: models.StatusUnknown,
			IsStale:      true,
			DataSource:   "live_prometheus",
		}
	}

	// Target is UP; check active alert or threshold severity
	if in.HasCriticalAlert ||
		(in.CPU != nil && *in.CPU >= DefaultCPUCritical) ||
		(in.Memory != nil && *in.Memory >= DefaultMemCritical) ||
		(in.DiskMax != nil && *in.DiskMax >= DefaultDiskCrit) {
		return DerivedStatus{
			Status:       models.StatusCritical,
			Availability: models.StatusUp,
			IsStale:      false,
			DataSource:   "live_prometheus",
		}
	}

	if in.HasWarningAlert ||
		(in.CPU != nil && *in.CPU >= DefaultCPUWarning) ||
		(in.Memory != nil && *in.Memory >= DefaultMemWarning) ||
		(in.DiskMax != nil && *in.DiskMax >= DefaultDiskWarning) {
		return DerivedStatus{
			Status:       models.StatusWarning,
			Availability: models.StatusUp,
			IsStale:      false,
			DataSource:   "live_prometheus",
		}
	}

	return DerivedStatus{
		Status:       models.StatusUp,
		Availability: models.StatusUp,
		IsStale:      false,
		DataSource:   "live_prometheus",
	}
}

// UnweightedAverage computes an unweighted average over non-nil valid values and returns
// the average (or nil if none available) alongside the number of included machines.
func UnweightedAverage(values []*float64) (*float64, int) {
	sum := 0.0
	count := 0
	for _, v := range values {
		if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
			continue
		}
		sum += *v
		count++
	}
	if count == 0 {
		return nil, 0
	}
	avg := math.Round((sum/float64(count))*100) / 100
	return &avg, count
}

// ParseTimeRange translates named dashboard ranges or custom RFC3339 timestamps into [start, end, step].
func ParseTimeRange(rangeKey, startStr, endStr string) (time.Time, time.Time, time.Duration) {
	now := time.Now().UTC()
	if startStr != "" && endStr != "" {
		s, err1 := time.Parse(time.RFC3339, startStr)
		e, err2 := time.Parse(time.RFC3339, endStr)
		if err1 == nil && err2 == nil && e.After(s) {
			if e.Sub(s) > 31*24*time.Hour {
				s = e.Add(-31 * 24 * time.Hour)
			}
			return s.UTC(), e.UTC(), chooseStep(e.Sub(s))
		}
	}

	var dur time.Duration
	switch rangeKey {
	case "15m":
		dur = 15 * time.Minute
	case "30m":
		dur = 30 * time.Minute
	case "1h":
		dur = time.Hour
	case "6h":
		dur = 6 * time.Hour
	case "24h":
		dur = 24 * time.Hour
	case "7d":
		dur = 7 * 24 * time.Hour
	case "30d":
		dur = 30 * 24 * time.Hour
	default:
		dur = time.Hour
	}

	start := now.Add(-dur)
	return start, now, chooseStep(dur)
}

func chooseStep(dur time.Duration) time.Duration {
	switch {
	case dur <= 30*time.Minute:
		return 15 * time.Second
	case dur <= 2*time.Hour:
		return 30 * time.Second
	case dur <= 6*time.Hour:
		return 2 * time.Minute
	case dur <= 24*time.Hour:
		return 10 * time.Minute
	case dur <= 7*24*time.Hour:
		return time.Hour
	default:
		return 4 * time.Hour
	}
}
