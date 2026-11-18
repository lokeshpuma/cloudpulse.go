package notifications

import (
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
)

var envRefRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)

// ValidateChannelConfig validates channel configuration references securely.
// Secrets must be supplied as environment/secret references (e.g. "env:SLACK_WEBHOOK_URL")
// or validated public HTTPS URLs (for non-secret target summary) while blocking SSRF targets.
func ValidateChannelConfig(channelType, configRef, targetSummary string) error {
	ct := strings.ToLower(strings.TrimSpace(channelType))
	ref := strings.TrimSpace(configRef)
	if ref == "" {
		return errors.New("configuration_reference is required")
	}

	// Preferred secure mechanism: environment variable or secret manager reference
	if strings.HasPrefix(ref, "env:") {
		envName := strings.TrimPrefix(ref, "env:")
		if !envRefRe.MatchString(envName) {
			return errors.New("environment secret reference must match env:UPPER_SNAKE_CASE")
		}
		return nil
	}
	if strings.HasPrefix(ref, "secret:") {
		key := strings.TrimSpace(strings.TrimPrefix(ref, "secret:"))
		if key == "" || strings.ContainsAny(key, " \t\n\r") {
			return errors.New("invalid secret reference")
		}
		return nil
	}

	switch ct {
	case "email":
		if _, err := mail.ParseAddress(ref); err != nil {
			return fmt.Errorf("email channel requires a valid email address or env:/secret: reference: %w", err)
		}
		return nil
	case "slack", "webhook":
		return ValidateOutboundWebhookURL(ref)
	default:
		return errors.New("channel_type must be email, slack, or webhook")
	}
}

// ValidateOutboundWebhookURL prevents SSRF by requiring HTTPS and blocking loopback,
// link-local, metadata, and private IP literals.
func ValidateOutboundWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	if u.Scheme != "https" {
		return errors.New("outbound webhook URLs must use https:// (or use an env: reference)")
	}
	if u.Host == "" {
		return errors.New("webhook URL must contain a host")
	}
	if u.User != nil {
		return errors.New("webhook URL must not embed credentials; use env: or secret: references")
	}

	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return errors.New("webhook host cannot target localhost or internal hostnames")
	}

	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return errors.New("webhook host cannot target private, loopback, or link-local IP addresses")
		}
	}
	return nil
}
