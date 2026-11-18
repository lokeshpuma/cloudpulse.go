# CloudPulse Go Monitoring Agent (`cloudpulse-agent`)

Standalone, low-overhead Go monitoring agent collecting real host CPU, memory, disk, network, and HTTP/TCP service availability metrics in Prometheus exposition format.

## Commands

```bash
go mod download
go fmt ./...
go vet ./...
go test -race -v ./...
go build -buildvcs=false -o ../bin/cloudpulse-agent ./cmd/agent
../bin/cloudpulse-agent --config ./config.example.yaml
```

## Endpoints
- `GET /metrics` — Prometheus exposition format
- `GET /health` — Collector & service check health JSON
- `GET /agent/info` — Agent version, hostname, OS, architecture, uptime metadata
