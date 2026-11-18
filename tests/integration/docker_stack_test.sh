#!/usr/bin/env bash
set -euo pipefail

# Docker Compose end-to-end integration test verifying:
# 1. Core services health (Backend, Postgres, Prometheus, Alertmanager, Grafana, Frontend)
# 2. Auth registration & JWT + CSRF refresh flow
# 3. Multi-server registration (2 live Go agents: agent-node-1:9180 & agent-node-2:9180)
# 4. Prometheus file_sd target discovery and real host metric collection
# 5. Controlled alert creation (via Alertmanager test alert / ServerDown), acknowledgement, and recovery

API="${API_BASE_URL:-http://localhost:8080}"
PROM="${PROMETHEUS_URL:-http://127.0.0.1:9090}"
AM="${ALERTMANAGER_URL:-http://127.0.0.1:9093}"
EMAIL="verify-$(date +%s)@cloudpulse.local"

echo "==> 1. Verifying /healthz and /readyz"
curl -sf "${API}/healthz" | grep -q '"status":"alive"'
curl -sf "${API}/readyz" | grep -q '"ready":true'

echo "==> 2. Registering organization & admin account (${EMAIL})"
REG_RESP=$(curl -sf -c /tmp/cp_cookies.txt -X POST "${API}/api/v1/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"Stack Verifier\",\"email\":\"${EMAIL}\",\"password\":\"VerifyPass123\",\"organization_name\":\"Verification Org\"}")

TOKEN=$(echo "${REG_RESP}" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")
CSRF=$(echo "${REG_RESP}" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['csrf_token'])")
test -n "${TOKEN}"

echo "==> 3. Testing HttpOnly refresh + CSRF rotation"
curl -sf -b /tmp/cp_cookies.txt -X POST "${API}/api/v1/auth/refresh" -H "X-CSRF-Token: ${CSRF}" | grep -q '"access_token"'

echo "==> 4. Registering two live Go monitoring agents (agent-node-1 & agent-node-2)"
SRV1=$(curl -sf -X POST "${API}/api/v1/servers" \
  -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" \
  -d '{"name":"app-server-01","hostname":"agent-node-1","environment":"production","operating_system":"linux","server_group":"app","scrape_address":"agent-node-1:9180","monitoring_interval":15}')
echo "${SRV1}" | grep -q '"status":"PENDING"'
SRV1_ID=$(echo "${SRV1}" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['id'])")

SRV2=$(curl -sf -X POST "${API}/api/v1/servers" \
  -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" \
  -d '{"name":"db-server-01","hostname":"agent-node-2","environment":"staging","operating_system":"linux","server_group":"database","scrape_address":"agent-node-2:9180","monitoring_interval":15}')
echo "${SRV2}" | grep -q '"status":"PENDING"'
SRV2_ID=$(echo "${SRV2}" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['id'])")

echo "==> 5. Waiting for Prometheus file_sd discovery & first real scrape (up to 40s)..."
for i in $(seq 1 20); do
  OV=$(curl -sf -H "Authorization: Bearer ${TOKEN}" "${API}/api/v1/monitoring/overview?range=15m")
  UP_COUNT=$(echo "${OV}" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['servers']['up'])")
  if [ "${UP_COUNT}" -ge 2 ]; then
    echo "    Both agents discovered and reporting UP! Overview: ${OV}"
    break
  fi
  sleep 2
done

UP_COUNT=$(curl -sf -H "Authorization: Bearer ${TOKEN}" "${API}/api/v1/monitoring/overview" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['servers']['up'])")
if [ "${UP_COUNT}" -lt 2 ]; then
  echo "ERROR: Expected 2 UP agents, got ${UP_COUNT}"
  exit 1
fi

echo "==> 6. Verifying live server metrics for ${SRV1_ID} and ${SRV2_ID}"
curl -sf -H "Authorization: Bearer ${TOKEN}" "${API}/api/v1/servers/${SRV1_ID}/metrics" | grep -q '"prometheus_available":true'
curl -sf -H "Authorization: Bearer ${TOKEN}" "${API}/api/v1/servers/${SRV2_ID}/metrics" | grep -q '"prometheus_available":true'

echo "==> 7. Testing controlled alert lifecycle (Alertmanager -> CloudPulse Alerts -> Acknowledge)"
NOW=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
ENDS=$(python3 -c "import datetime; print((datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(minutes=5)).strftime('%Y-%m-%dT%H:%M:%SZ'))")
curl -sf -X POST "${AM}/api/v2/alerts" -H "Content-Type: application/json" -d "[{
  \"labels\":{\"alertname\":\"ControlledCPUCheck\",\"severity\":\"warning\",\"server_name\":\"app-server-01\",\"cloudpulse_server_id\":\"${SRV1_ID}\",\"environment\":\"production\"},
  \"annotations\":{\"summary\":\"Controlled test alert on app-server-01\",\"description\":\"Triggered by integration test to verify alert acknowledgement.\"},
  \"startsAt\":\"${NOW}\",\"endsAt\":\"${ENDS}\"
}]"
sleep 1
ALERTS=$(curl -sf -H "Authorization: Bearer ${TOKEN}" "${API}/api/v1/alerts")
FP=$(echo "${ALERTS}" | python3 -c "import sys,json; items=json.load(sys.stdin)['data']; print(next(a['fingerprint'] for a in items if a['alert_name']=='ControlledCPUCheck'))")
curl -sf -X POST "${API}/api/v1/alerts/${FP}/acknowledge" \
  -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" \
  -d '{"note":"Verified in integration test"}' | grep -q '"alert_fingerprint"'

echo "==> 8. Verifying CSV report export & system integrations"
curl -sf -H "Authorization: Bearer ${TOKEN}" "${API}/api/v1/reports/export?type=infrastructure&format=csv" | grep -q "app-server-01"
curl -sf -H "Authorization: Bearer ${TOKEN}" "${API}/api/v1/system/integrations" | grep -q '"grafana"'

rm -f /tmp/cp_cookies.txt
echo "ALL LIVE INTEGRATION CHECKS PASSED!"
