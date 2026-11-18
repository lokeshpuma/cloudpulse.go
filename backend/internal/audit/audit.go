package audit

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

// Writer persists append-only audit records.
type Writer interface {
	InsertAuditLog(ctx context.Context, orgID uuid.UUID, actorUserID *uuid.UUID, action, resourceType, resourceID, sourceIP string, metadata map[string]any) error
}

// Recorder sanitizes sensitive metadata and writes append-only audit records.
type Recorder struct {
	writer Writer
	logger *slog.Logger
}

func NewRecorder(writer Writer, logger *slog.Logger) *Recorder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Recorder{writer: writer, logger: logger}
}

var sensitiveKeyFragments = []string{
	"password", "secret", "token", "credential", "authorization", "cookie", "webhook_url", "api_key",
}

// SanitizeMetadata strips any sensitive fields so secrets are never persisted in audit logs.
func SanitizeMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		lower := strings.ToLower(k)
		redacted := false
		for _, frag := range sensitiveKeyFragments {
			if strings.Contains(lower, frag) {
				redacted = true
				break
			}
		}
		if redacted {
			out[k] = "[REDACTED]"
		} else {
			out[k] = v
		}
	}
	return out
}

func (r *Recorder) Record(ctx context.Context, orgID uuid.UUID, actorUserID *uuid.UUID, action, resourceType, resourceID, sourceIP string, metadata map[string]any) {
	if r == nil || r.writer == nil {
		return
	}
	safe := SanitizeMetadata(metadata)
	if err := r.writer.InsertAuditLog(ctx, orgID, actorUserID, action, resourceType, resourceID, sourceIP, safe); err != nil {
		r.logger.Error("failed to record audit log", "action", action, "resource_type", resourceType, "error", err)
	}
}
