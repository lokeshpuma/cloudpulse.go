# syntax=docker/dockerfile:1.7
FROM golang:1.26-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG AGENT_VERSION=1.0.0
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -trimpath -ldflags="-s -w -X main.Version=${AGENT_VERSION}" -o /out/cloudpulse-agent ./cmd/agent

FROM alpine:3.21.3
RUN apk add --no-cache ca-certificates wget \
    && addgroup -S cloudpulse \
    && adduser -S -G cloudpulse -u 10002 cloudpulse
WORKDIR /app
COPY --from=builder /out/cloudpulse-agent /usr/local/bin/cloudpulse-agent
COPY config.example.yaml /etc/cloudpulse/config.yaml
RUN chown -R cloudpulse:cloudpulse /app /etc/cloudpulse
USER cloudpulse
EXPOSE 9180
HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:9180/health || exit 1
ENTRYPOINT ["/usr/local/bin/cloudpulse-agent"]
CMD ["--config", "/etc/cloudpulse/config.yaml"]
