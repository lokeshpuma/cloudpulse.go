# Prometheus Integration

Prometheus is CloudPulse's sole time-series database and alert evaluation engine.

## Configuration

Main configuration: [infrastructure/prometheus/prometheus.yml](../infrastructure/prometheus/prometheus.yml)

- Self-monitors `prometheus:9090`.
- Discovers registered `cloudpulse-agent` instances via `file_sd_configs` reading `/etc/prometheus/file_sd/*.json` every 15s.
- Loads default and custom alert rules from `/etc/prometheus/rules/*.yml`.
- Configured with `--web.enable-lifecycle` and `--storage.tsdb.retention.time=30d`.

## Atomic Target Synchronization

Whenever an Administrator registers, edits, enables, disables, or removes a server in CloudPulse, the Go backend (`backend/internal/discovery`):
1. Queries all `monitoring_enabled = TRUE` servers from PostgreSQL.
2. Serializes Prometheus `file_sd` target groups with labels `cloudpulse_server_id`, `cloudpulse_org_id`, `server_name`, `hostname`, `environment`, `server_group`, and `operating_system`.
3. Writes to a temporary file in `/etc/prometheus/file_sd/` and atomically `os.Rename()`s it into `targets.json` so Prometheus never reads a half-written JSON file.
4. Sends `POST /-/reload` to Prometheus when `PROMETHEUS_RELOAD_ENABLED=true`.

## Core Metric Names & PromQL Patterns

- **CPU:** `cloudpulse_cpu_usage_percent`, `cloudpulse_cpu_core_usage_percent`, `cloudpulse_cpu_core_count`, `cloudpulse_cpu_load_1m|5m|15m`
- **Memory:** `cloudpulse_memory_total_bytes`, `cloudpulse_memory_used_bytes`, `cloudpulse_memory_available_bytes`, `cloudpulse_memory_usage_percent`, `cloudpulse_memory_cached_bytes`, `cloudpulse_memory_free_bytes`
- **Disk:** `cloudpulse_disk_total_bytes`, `cloudpulse_disk_used_bytes`, `cloudpulse_disk_free_bytes`, `cloudpulse_disk_usage_percent` (labels: `mountpoint`, `device`, `filesystem`)
- **Network (cumulative counters — always use `rate()`):**
  - `rate(cloudpulse_network_receive_bytes_total[5m])`
  - `rate(cloudpulse_network_transmit_bytes_total[5m])`
  - `rate(cloudpulse_network_receive_errors_total[5m])`
- **Services:** `cloudpulse_service_up`, `cloudpulse_service_response_duration_seconds`, `cloudpulse_service_check_errors_total`, `cloudpulse_service_http_status_code`, `cloudpulse_service_tls_expiry_seconds`
- **Availability:** `up{job="cloudpulse-agents"}`
