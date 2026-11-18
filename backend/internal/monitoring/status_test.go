package monitoring

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cloudpulse/cloudpulse-go/backend/internal/models"
)

func ptr(v float64) *float64 { return &v }

func TestDeriveServerStatusRules(t *testing.T) {
	now := time.Now().UTC()
	fresh := now.Add(-15 * time.Second)
	stale := now.Add(-5 * time.Minute)

	t.Run("newly registered server is PENDING, never UP", func(t *testing.T) {
		d := DeriveServerStatus(ServerLiveInput{
			MonitoringEnabled: true,
			PrometheusOnline:  true,
			Now:               now,
		})
		assert.Equal(t, models.StatusPending, d.Status)
		assert.Equal(t, models.StatusUnknown, d.Availability)
	})

	t.Run("prometheus outage yields UNKNOWN unavailable", func(t *testing.T) {
		d := DeriveServerStatus(ServerLiveInput{
			MonitoringEnabled: true,
			PrometheusOnline:  false,
			Now:               now,
		})
		assert.Equal(t, models.StatusUnknown, d.Status)
		assert.Equal(t, "unavailable", d.DataSource)
		assert.True(t, d.IsStale)
	})

	t.Run("stale metric is UNKNOWN with IsStale=true, never UP", func(t *testing.T) {
		d := DeriveServerStatus(ServerLiveInput{
			MonitoringEnabled: true,
			PrometheusOnline:  true,
			UpSample:          ptr(1),
			LastSampleAt:      &stale,
			Now:               now,
		})
		assert.Equal(t, models.StatusUnknown, d.Status)
		assert.True(t, d.IsStale)
	})

	t.Run("down target is DOWN", func(t *testing.T) {
		d := DeriveServerStatus(ServerLiveInput{
			MonitoringEnabled: true,
			PrometheusOnline:  true,
			UpSample:          ptr(0),
			TargetHealth:      "down",
			LastSampleAt:      &fresh,
			Now:               now,
		})
		assert.Equal(t, models.StatusDown, d.Status)
		assert.Equal(t, models.StatusDown, d.Availability)
	})

	t.Run("fresh healthy target is UP", func(t *testing.T) {
		d := DeriveServerStatus(ServerLiveInput{
			MonitoringEnabled: true,
			PrometheusOnline:  true,
			UpSample:          ptr(1),
			TargetHealth:      "up",
			LastSampleAt:      &fresh,
			CPU:               ptr(35),
			Memory:            ptr(50),
			DiskMax:           ptr(40),
			Now:               now,
		})
		assert.Equal(t, models.StatusUp, d.Status)
		assert.Equal(t, models.StatusUp, d.Availability)
		assert.False(t, d.IsStale)
	})

	t.Run("warning and critical thresholds escalate status", func(t *testing.T) {
		warn := DeriveServerStatus(ServerLiveInput{
			MonitoringEnabled: true, PrometheusOnline: true, UpSample: ptr(1), LastSampleAt: &fresh, CPU: ptr(80), Now: now,
		})
		assert.Equal(t, models.StatusWarning, warn.Status)

		crit := DeriveServerStatus(ServerLiveInput{
			MonitoringEnabled: true, PrometheusOnline: true, UpSample: ptr(1), LastSampleAt: &fresh, CPU: ptr(95), Now: now,
		})
		assert.Equal(t, models.StatusCritical, crit.Status)
	})
}

func TestUnweightedAverageIgnoresNil(t *testing.T) {
	avg, count := UnweightedAverage([]*float64{ptr(20), nil, ptr(40), nil})
	assert.Equal(t, 2, count)
	assert.NotNil(t, avg)
	assert.Equal(t, 30.0, *avg)

	emptyAvg, emptyCount := UnweightedAverage([]*float64{nil, nil})
	assert.Equal(t, 0, emptyCount)
	assert.Nil(t, emptyAvg)
}
