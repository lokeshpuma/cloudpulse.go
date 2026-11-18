# Alerting & Notifications Guide

CloudPulse uses **Prometheus** for rule evaluation and **Alertmanager** for alert routing, grouping, inhibition, and deduplication. CloudPulse never implements a competing notification router in Go.

## Default Alert Rules

Provisioned in [infrastructure/prometheus/rules/infrastructure.yml](../infrastructure/prometheus/rules/infrastructure.yml) and [infrastructure/prometheus/rules/services.yml](../infrastructure/prometheus/rules/services.yml), and seeded per organization in PostgreSQL so administrators can customize thresholds or disable rules:

| Rule | Default Condition | Duration | Severity |
|---|---|---|---|
| CPU Usage Warning | `cloudpulse_cpu_usage_percent > 75` | `5m` | Warning |
| CPU Usage Critical | `cloudpulse_cpu_usage_percent > 90` | `5m` | Critical |
| Memory Usage Warning | `cloudpulse_memory_usage_percent > 80` | `5m` | Warning |
| Memory Usage Critical | `cloudpulse_memory_usage_percent > 95` | `5m` | Critical |
| Disk Usage Warning | `cloudpulse_disk_usage_percent > 80` | `5m` | Warning |
| Disk Usage Critical | `cloudpulse_disk_usage_percent > 90` | `5m` | Critical |
| Server Down | `up{job="cloudpulse-agents"} == 0` | `1m` | Critical |
| Service Down | `cloudpulse_service_up == 0` | `2m` | Critical |
| High Network Errors | `rate(rx_err[5m]) + rate(tx_err[5m]) > 5` | `5m` | Warning |

## Alert Acknowledgement

Acknowledging an alert via `POST /api/v1/alerts/{fingerprint}/acknowledge` stores the operator's `user_id`, timestamp, and note in PostgreSQL (`alert_acknowledgements`). It intentionally does **not** silence or resolve the alert in Prometheus so operators continue to observe actual condition recovery.

## Configuring Production Email, Slack, and Webhook Receivers

Edit [infrastructure/alertmanager/alertmanager.yml](../infrastructure/alertmanager/alertmanager.yml) and mount credentials via Docker secrets (`/run/secrets/...`):

- **Slack:** Use `slack_configs[].api_url_file: "/run/secrets/slack_webhook_url"`
- **Email (SMTP):** Use `email_configs[].auth_password_file: "/run/secrets/smtp_password"`
- **Generic Webhook:** Use `webhook_configs[].url` pointing to an authenticated HTTPS receiver

In CloudPulse → **Notifications**, register channel references using `env:SLACK_WEBHOOK_URL` or `secret:...` rather than storing raw secrets in database records. Use **Test via Alertmanager** to verify Alertmanager dispatch connectivity.
