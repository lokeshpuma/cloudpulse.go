# CloudPulse REST API Reference

Base prefix: `/api/v1` (plus `/healthz` and `/readyz`).
Full OpenAPI 3.1 specification: [backend/openapi.yaml](../backend/openapi.yaml).

## Response Envelope

Success:
```json
{
  "success": true,
  "data": {},
  "message": "Request completed successfully",
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

Error:
```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid request"
  },
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

## Authentication & RBAC Matrix

| Role | Permissions |
|---|---|
| `admin` | Full access: users, organization settings, servers CRUD, alert rules CRUD, notification channels CRUD, audit logs, reports |
| `operator` | View all dashboards/metrics/servers, acknowledge alerts, manage service checks, view audit logs, export reports |
| `viewer` | Read-only access to dashboards, servers, metrics explorer, alerts, and reports |

Cookie-authenticated session renewal (`POST /api/v1/auth/refresh`) and logout (`POST /api/v1/auth/logout`) require the `X-CSRF-Token` header matching the `cloudpulse_csrf` cookie.

## Endpoints Summary

- **Auth:** `POST /auth/register`, `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`, `POST /auth/forgot-password`, `POST /auth/reset-password`, `GET /auth/me`
- **Organization & Users:** `GET/PATCH /organizations/current`, `GET/POST /users`, `PATCH /users/{id}`, `POST /users/{id}/disable`, `POST /users/{id}/reset-access`, `GET /audit-logs`
- **Servers:** `GET/POST /servers`, `GET/PATCH/DELETE /servers/{id}`, `POST /servers/{id}/enable`, `POST /servers/{id}/disable`, `GET /servers/{id}/status`, `GET /servers/{id}/metrics`, `GET /servers/{id}/history`
- **Monitoring:** `GET /monitoring/overview`, `GET /monitoring/health`, `GET /monitoring/metrics`, `GET /monitoring/timeseries`, `GET /monitoring/targets`
- **Services:** `GET/POST /services`, `GET/PATCH/DELETE /services/{id}`, `GET /services/{id}/history`
- **Alerts:** `GET /alerts`, `GET /alerts/history`, `GET /alerts/{fingerprint}`, `POST /alerts/{fingerprint}/acknowledge`
- **Alert Rules:** `GET/POST /alert-rules`, `GET/PATCH/DELETE /alert-rules/{id}`, `POST /alert-rules/{id}/validate`
- **Notifications:** `GET /notifications`, `PATCH /notifications/{id}/read`, `POST /notifications/read-all`, `GET/POST /notification-channels`, `PATCH/DELETE /notification-channels/{id}`, `POST /notification-channels/{id}/test`
- **Reports:** `GET /reports/infrastructure`, `GET /reports/resources`, `GET /reports/services`, `GET /reports/alerts`, `GET /reports/export?type=...&format=csv|json|html`
- **System:** `GET /healthz`, `GET /readyz`, `GET /api/v1/system/version`, `GET /api/v1/system/integrations`
