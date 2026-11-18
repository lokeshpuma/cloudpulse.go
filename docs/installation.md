# Local Installation & Development Guide

## Prerequisites

- Go 1.24+ (developed and tested on Go 1.26)
- Node.js 22+ and npm
- Docker and Docker Compose v2+
- Git

## Quick Start with Docker Compose

```bash
cp .env.example .env
docker compose up -d --build
```

To also launch two live local `cloudpulse-agent` containers for multi-server testing:

```bash
docker compose --profile agents up -d --build
```

Services started:
- **CloudPulse Dashboard (Nginx + React):** http://localhost:3000
- **Go Backend API:** http://localhost:8080
- **Grafana (provisioned dashboards):** http://localhost:3001
- **Prometheus (loopback only):** http://127.0.0.1:9090
- **Alertmanager (loopback only):** http://127.0.0.1:9093
- **PostgreSQL:** internal Docker network only (never exposed publicly)

## Local Hybrid Development (Hot Reload)

1. Start supporting infrastructure containers:
   ```bash
   docker compose up -d postgres prometheus-init prometheus grafana alertmanager
   ```

2. Run the Go backend API locally:
   ```bash
   cd backend
   go mod download
   DATABASE_URL="postgres://cloudpulse:change-me-strong-db-password@127.0.0.1:5432/cloudpulse?sslmode=disable" \
   JWT_SECRET="local-dev-secret-key-at-least-32-characters-long" \
   PROMETHEUS_URL="http://127.0.0.1:9090" \
   ALERTMANAGER_URL="http://127.0.0.1:9093" \
   GRAFANA_URL="http://127.0.0.1:3001" \
   PROMETHEUS_FILE_SD_PATH="../infrastructure/prometheus/file_sd/targets.json" \
   PROMETHEUS_RULES_DIR="../infrastructure/prometheus/rules" \
   go run ./cmd/api
   ```
   *(If connecting from host to Docker PostgreSQL during local Go development, temporarily map `127.0.0.1:5432:5432` in a local `docker-compose.override.yml` — never expose 5432 publicly.)*

3. Run the React + TypeScript frontend dev server:
   ```bash
   cd frontend
   cp .env.example .env
   npm install
   npm run dev
   ```
   Open http://localhost:5173.

4. Run a local standalone Go agent:
   ```bash
   cd agent
   go run ./cmd/agent --config ./config.example.yaml
   ```
   Verify:
   ```bash
   curl http://127.0.0.1:9180/health
   curl http://127.0.0.1:9180/agent/info
   curl http://127.0.0.1:9180/metrics | head -n 30
   ```
