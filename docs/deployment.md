# Production & GitHub Pages Deployment Guide

CloudPulse supports two deployment topologies:

1. **Full Self-Hosted Docker Stack** (VPS, Cloud VM, private server)
2. **GitHub Pages Static Frontend** pointing either to your HTTPS-hosted Go backend OR running in explicitly marked public **Demo Mode**

---

## 1. Docker Compose Self-Hosting (VPS / Cloud VM)

### Security Checklist Before Starting

1. Copy `.env.example` to `.env` and set strong random credentials:
   - `APP_ENV=production`
   - `COOKIE_SECURE=true`
   - `JWT_SECRET` — minimum 32 random characters (cannot contain `change-me`)
   - `POSTGRES_PASSWORD` — strong unique password
   - `GRAFANA_ADMIN_PASSWORD` — strong unique password
   - `CORS_ALLOWED_ORIGINS` — exact HTTPS origin(s), e.g. `https://monitor.example.com` (never `*`)
2. Place an HTTPS-terminating reverse proxy (Nginx + Let's Encrypt / Caddy / Traefik) in front of port `3000` (frontend) and `8080` (API) / `3001` (Grafana).
3. Configure host firewall (`ufw` / cloud security groups):
   - Allow `80/tcp` and `443/tcp` publicly for HTTPS termination.
   - Keep `5432` (PostgreSQL), `9090` (Prometheus), `9093` (Alertmanager), and `9180` (Agents) strictly private or VPN-only.

### Lifecycle Commands

```bash
# Build and start all services detached
docker compose up -d --build

# Inspect running service status and healthchecks
docker compose ps

# Follow aggregated logs
docker compose logs -f

# Stop services WITHOUT removing persistent volumes
docker compose down
```

Persistent named volumes (`postgres_data`, `prometheus_data`, `grafana_data`, `alertmanager_data`, `prometheus_file_sd`, `prometheus_rules`) are preserved across restarts and `docker compose down`.

---

## 2. GitHub Pages Frontend Deployment

GitHub Pages serves static HTML/JS/CSS only; it cannot run Go, PostgreSQL, Prometheus, Grafana, or Alertmanager.

### Option A: Public Demo Mode on GitHub Pages
The included workflow [deploy-pages.yml](../.github/workflows/deploy-pages.yml) defaults to:
```env
GITHUB_PAGES=true
VITE_API_BASE_URL=
VITE_APP_ENV=demo
VITE_DEMO_MODE=true
```
Every dashboard screen clearly marks fixture values as `DEMO` and never claims they are live metrics.

### Option B: GitHub Pages Connected to a Self-Hosted HTTPS Backend
In your GitHub repository **Settings → Secrets and variables → Actions → Variables**, set:
- `VITE_API_BASE_URL=https://api.monitor.example.com`
- `VITE_APP_ENV=production`
- `VITE_DEMO_MODE=false`

Configure `CORS_ALLOWED_ORIGINS=https://<username>.github.io` and `COOKIE_SECURE=true` on your HTTPS Go backend. Enable GitHub Pages with **Source: GitHub Actions**.

**Never place passwords, JWT secrets, or webhook tokens in `VITE_*` variables**, as Vite bundles them into public browser JavaScript.
