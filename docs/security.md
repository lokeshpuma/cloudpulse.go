# CloudPulse Security Architecture & Hardening

## Authentication & Session Security
- Passwords are validated server-side (>=8 chars, upper, lower, digit) and hashed with **bcrypt**.
- Short-lived **JWT access tokens** (`15m` default) are stored strictly in browser memory — never in `localStorage` or `sessionStorage`.
- Long-lived **refresh tokens** are stored as SHA-256 hashes in PostgreSQL (`refresh_sessions`), transmitted via `HttpOnly`, `SameSite`, `Secure` (in production) cookies, and rotated on every refresh.
- State-changing cookie endpoints (`/api/v1/auth/refresh`, `/api/v1/auth/logout`) enforce double-submit **CSRF** verification (`X-CSRF-Token` vs `cloudpulse_csrf`).
- Auth endpoints are IP rate-limited and return generic login failure messages to prevent user enumeration.

## Authorization & Tenant Isolation
- Role-based access control (`admin`, `operator`, `viewer`) is enforced on every Go backend route independently of frontend UI visibility.
- Every database query scopes resources by `organization_id` from the verified JWT claims.

## Backend & Input Hardening
- Parameterized SQL queries via `pgx/v5` prevent SQL injection.
- Request payload size is capped (`2 MiB`) and outbound external calls use bounded timeouts.
- Notification channel validation blocks SSRF targets (loopback, private RFC1918, link-local metadata `169.254.169.254`, HTTP non-TLS, embedded credentials).
- CSV report exports escape leading `=`, `+`, `-`, `@`, `\t`, `\r` characters to prevent spreadsheet formula injection (CWE-1236).
- Audit logs are append-only and automatically redact keys matching `password`, `secret`, `token`, `credential`, `authorization`, `cookie`, `webhook_url`, or `api_key`.
- Production configuration loader (`backend/internal/config`) refuses startup if `JWT_SECRET` or `DATABASE_URL` contain `change-me`, `COOKIE_SECURE=false`, or wildcard CORS `*`.

## Infrastructure & Agent Hardening
- PostgreSQL runs exclusively on the internal Docker network (`cloudpulse_internal`) with no public host port.
- Prometheus (`127.0.0.1:9090`) and Alertmanager (`127.0.0.1:9093`) bind to loopback only by default.
- Backend, frontend, and agent Dockerfiles use pinned versions and run as non-root users.
- `cloudpulse-agent` supports environment-supplied Bearer token auth and mutual TLS (`tls_client_ca`), strips credentials/query params from HTTP service target labels, and ships with a hardened non-root systemd unit.
