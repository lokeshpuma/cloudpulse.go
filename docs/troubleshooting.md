# CloudPulse Troubleshooting Guide

## 1. Newly Registered Server Stays in `PENDING` Status
- **Expected behavior:** CloudPulse never marks a server `UP` until Prometheus successfully scrapes it.
- **Check:**
  1. Confirm the agent is running: `curl http://<agent-host>:9180/health` and `curl http://<agent-host>:9180/metrics`.
  2. Verify Prometheus can reach the `scrape_address` from inside the Prometheus container/network (`http://127.0.0.1:9090/targets`).
  3. Confirm `targets.json` contains your server inside the shared `prometheus_file_sd` volume.

## 2. Dashboard Shows "Monitoring Data Unavailable"
- Prometheus is unreachable or restarting. CloudPulse intentionally avoids fabricating numbers during outages.
- Inspect Prometheus logs: `docker compose logs -f prometheus`.
- Verify `GET http://127.0.0.1:9090/-/healthy` returns `Prometheus Server is Healthy.`.

## 3. Backend Refuses to Start in Production Mode
- Check `docker compose logs backend`.
- When `APP_ENV=production`, `JWT_SECRET` must be >=32 characters without `change-me`, `DATABASE_URL` must not contain `change-me`, `COOKIE_SECURE` must be `true`, and `CORS_ALLOWED_ORIGINS` must not include `*`.

## 4. Browser Refresh Returns 401/403 After Login
- Ensure `CORS_ALLOWED_ORIGINS` matches your browser's exact origin (scheme + host + port).
- Ensure your request includes `credentials: "include"` and `X-CSRF-Token` matching the `cloudpulse_csrf` cookie.

## 5. Grafana Shows Empty Panels
- Confirm Prometheus has scraped at least one `cloudpulse-agent` target (`up{job="cloudpulse-agents"} == 1`).
- Check **Settings → Integrations** in CloudPulse to verify Grafana and Prometheus health.

## 6. macOS Local `go build` VCS Error
- If `go build` fails locally with `error obtaining VCS status`, build with `-buildvcs=false` (already included in `scripts/build-agent.sh` and Dockerfiles).
