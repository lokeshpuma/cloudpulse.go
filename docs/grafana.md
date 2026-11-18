# Grafana Integration

Grafana (`grafana/grafana:11.5.2`) is pre-provisioned in Docker Compose with the Prometheus data source and four CloudPulse dashboards.

## Provisioned Assets

- **Data Source:** [infrastructure/grafana/provisioning/datasources/prometheus.yml](../infrastructure/grafana/provisioning/datasources/prometheus.yml) (`uid: cloudpulse-prometheus`, `isDefault: true`)
- **Dashboard Provider:** [infrastructure/grafana/provisioning/dashboards/dashboards.yml](../infrastructure/grafana/provisioning/dashboards/dashboards.yml)
- **Dashboards:**
  1. `infrastructure-overview.json` — Total targets, active servers, availability %, active alerts, CPU/Memory/Disk fleet timelines
  2. `server-performance.json` — CPU usage, load averages (1m/5m/15m), memory used vs available, disk by mountpoint, Rx/Tx throughput
  3. `service-monitoring.json` — Service UP/DOWN status, HTTP status codes, response latency, failed check rates, availability history
  4. `network-monitoring.json` — Rx/Tx bytes/sec, packet rates, interface errors

All four dashboards define template variables `environment` and `server` so CloudPulse can deep-link directly to filtered server views (e.g. `?var-server=app-server-01&var-environment=production`).

## Security Posture

- Anonymous access is explicitly disabled (`GF_AUTH_ANONYMOUS_ENABLED=false`).
- Self-signup is disabled (`GF_USERS_ALLOW_SIGN_UP=false`).
- CloudPulse never exposes Grafana administrator credentials to the browser frontend.
- A Grafana outage does **not** impact CloudPulse's core monitoring UI or Prometheus API queries.
