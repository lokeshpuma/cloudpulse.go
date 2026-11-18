package alertmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Alert represents an Alertmanager v2 alert object.
type Alert struct {
	Fingerprint string            `json:"fingerprint"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	Status      struct {
		State       string   `json:"state"` // active, suppressed, unprocessed
		SilencedBy  []string `json:"silencedBy"`
		InhibitedBy []string `json:"inhibitedBy"`
	} `json:"status"`
}

// Client abstracts Alertmanager HTTP API v2 interactions.
type Client interface {
	Healthy(ctx context.Context) error
	Alerts(ctx context.Context) ([]Alert, error)
	SendTestAlert(ctx context.Context, channelName, severity string) error
}

type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(baseURL string, timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HTTPClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *HTTPClient) Healthy(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/-/healthy", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("alertmanager returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *HTTPClient) Alerts(ctx context.Context) ([]Alert, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v2/alerts", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("alertmanager alerts error (%d)", resp.StatusCode)
	}

	var alerts []Alert
	if err := json.Unmarshal(body, &alerts); err != nil {
		return nil, fmt.Errorf("decode alertmanager alerts: %w", err)
	}
	return alerts, nil
}

// SendTestAlert dispatches a controlled test alert through Alertmanager's own routing pipeline
// so CloudPulse never builds a competing notification engine.
func (c *HTTPClient) SendTestAlert(ctx context.Context, channelName, severity string) error {
	if severity == "" {
		severity = "info"
	}
	now := time.Now().UTC()
	payload := []map[string]any{
		{
			"labels": map[string]string{
				"alertname": "CloudPulseNotificationTest",
				"severity":  severity,
				"channel":   channelName,
				"source":    "cloudpulse-test",
			},
			"annotations": map[string]string{
				"summary":     fmt.Sprintf("Test notification for channel %s", channelName),
				"description": "Controlled connectivity verification alert sent via Alertmanager.",
			},
			"startsAt": now.Format(time.RFC3339),
			"endsAt":   now.Add(2 * time.Minute).Format(time.RFC3339),
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/alerts", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("alertmanager test alert rejected (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
