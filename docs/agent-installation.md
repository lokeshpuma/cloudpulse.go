# CloudPulse Go Agent Installation Guide

`cloudpulse-agent` is a lightweight, standalone Go binary that collects real host CPU, memory, disk, network, and HTTP/TCP service metrics and exposes them in Prometheus exposition format.

## Endpoints

- `GET /metrics` — Prometheus exposition format (optionally protected by Bearer token and/or mTLS)
- `GET /health` — JSON collector and service check health status
- `GET /agent/info` — Agent version, hostname, OS, architecture, start time, and collection interval

## 1. Build the Standalone Binary

```bash
./scripts/build-agent.sh
```
Or build directly for Linux amd64:
```bash
cd agent
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -trimpath -o ../bin/cloudpulse-agent ./cmd/agent
```

## 2. Linux Systemd Installation (Recommended for Bare Metal / VPS)

```bash
# 1. Create dedicated non-root user
sudo useradd --system --no-create-home --shell /usr/sbin/nologin cloudpulse

# 2. Install binary and config directory
sudo install -m 0755 bin/cloudpulse-agent /usr/local/bin/cloudpulse-agent
sudo mkdir -p /etc/cloudpulse
sudo cp agent/config.example.yaml /etc/cloudpulse/config.yaml
sudo chown -R cloudpulse:cloudpulse /etc/cloudpulse
sudo chmod 640 /etc/cloudpulse/config.yaml

# 3. Edit /etc/cloudpulse/config.yaml
# Set unique agent.id, agent.name, agent.environment, bind listen_address to your private/VPN IP (e.g. 10.10.0.5:9180),
# and configure any local HTTP/TCP service checks.

# 4. Install and start systemd unit
sudo cp agent/deploy/cloudpulse-agent.service /etc/systemd/system/cloudpulse-agent.service
sudo systemctl daemon-reload
sudo systemctl enable --now cloudpulse-agent
sudo systemctl status cloudpulse-agent
```

## 3. Verification Commands

```bash
curl http://127.0.0.1:9180/health
curl http://127.0.0.1:9180/agent/info
curl http://127.0.0.1:9180/metrics | grep cloudpulse_
```

## 4. Running the Agent in Docker

`infrastructure/docker/agent.Dockerfile` builds a non-root container image (`USER cloudpulse`).
When collecting **host-level** metrics from a container on Linux, bind host filesystems read-only rather than using `--privileged`:

```yaml
services:
  cloudpulse-agent:
    build:
      context: ./agent
      dockerfile: ./infrastructure/docker/agent.Dockerfile
    user: "10002:10002"
    read_only: true
    network_mode: host
    volumes:
      - ./agent/config.example.yaml:/etc/cloudpulse/config.yaml:ro
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
    environment:
      HOST_PROC: /host/proc
      HOST_SYS: /host/sys
      AGENT_LISTEN_ADDRESS: "10.0.0.10:9180"
```

*Platform note:* On macOS/Windows Docker Desktop, containers run inside a Linux VM, so containerized agents report the Linux VM's CPU/memory/disk/network rather than the macOS/Windows host. Run the native binary on macOS/Windows to monitor the host directly.

## 5. Multi-Server Setup Across Private Networks / NAT

Never expose `:9180` to the public Internet without protection. For remote machines behind NAT:
1. Connect monitored hosts and your Prometheus server over a private mesh VPN (e.g., **WireGuard** or **Tailscale**).
2. Bind `server.listen_address` to the host's VPN IP (e.g. `100.64.0.12:9180`).
3. Restrict port `9180` via firewall (`ufw allow from <prometheus-vpn-ip> to any port 9180 proto tcp`).
4. Optionally enable `auth.bearer_token_env` (read from environment, never hard-coded) and/or `auth.tls_cert_file` + `auth.tls_client_ca` for mTLS.
5. Register the server's VPN address (`100.64.0.12:9180`) in CloudPulse → **Servers**.
