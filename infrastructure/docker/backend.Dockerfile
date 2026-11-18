# syntax=docker/dockerfile:1.7
FROM golang:1.26-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/cloudpulse-api ./cmd/api

FROM alpine:3.21.3
RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S cloudpulse \
    && adduser -S -G cloudpulse -u 10001 cloudpulse
WORKDIR /app
COPY --from=builder /out/cloudpulse-api /app/cloudpulse-api
RUN mkdir -p /etc/prometheus/file_sd /etc/prometheus/rules \
    && chown -R cloudpulse:cloudpulse /app /etc/prometheus
USER cloudpulse
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/cloudpulse-api"]
