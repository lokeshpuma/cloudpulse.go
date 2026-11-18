# CloudPulse Go Backend API

Layered Go HTTP API built with Chi v5, `pgx/v5`, JWT + HttpOnly refresh cookies, bcrypt, structured JSON logging (`slog`), atomic Prometheus `file_sd` synchronization, Alertmanager v2 integration, and Grafana link provisioning.

## Commands

```bash
go mod download
go fmt ./...
go vet ./...
go test -race -v ./...
go build -buildvcs=false -o ../bin/cloudpulse-api ./cmd/api
```

OpenAPI specification: [openapi.yaml](./openapi.yaml).
