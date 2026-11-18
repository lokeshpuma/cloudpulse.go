package grafana

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DashboardLink represents a safe user-facing link to a provisioned Grafana dashboard.
type DashboardLink struct {
	UID         string `json:"uid"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

// Client checks Grafana availability and constructs safe dashboard URLs without exposing credentials.
type Client struct {
	internalURL string
	publicURL   string
	httpClient  *http.Client
}

func NewClient(internalURL, publicURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	if strings.TrimSpace(publicURL) == "" {
		publicURL = internalURL
	}
	return &Client{
		internalURL: strings.TrimRight(internalURL, "/"),
		publicURL:   strings.TrimRight(publicURL, "/"),
		httpClient:  &http.Client{Timeout: timeout},
	}
}

func (c *Client) Healthy(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.internalURL+"/api/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("grafana returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) PublicURL() string {
	return c.publicURL
}

func (c *Client) DashboardLinks(serverName, environment string) []DashboardLink {
	q := url.Values{}
	if serverName != "" {
		q.Set("var-server", serverName)
	}
	if environment != "" {
		q.Set("var-environment", environment)
	}
	suffix := ""
	if encoded := q.Encode(); encoded != "" {
		suffix = "?" + encoded
	}

	return []DashboardLink{
		{
			UID:         "cloudpulse-infra-overview",
			Title:       "Infrastructure Overview",
			Description: "Total servers, availability, CPU/Memory/Disk fleet overview, and active alerts",
			URL:         fmt.Sprintf("%s/d/cloudpulse-infra-overview/infrastructure-overview%s", c.publicURL, suffix),
		},
		{
			UID:         "cloudpulse-server-perf",
			Title:       "Server Performance",
			Description: "Detailed CPU, load, memory, mountpoint disk usage, and network throughput",
			URL:         fmt.Sprintf("%s/d/cloudpulse-server-perf/server-performance%s", c.publicURL, suffix),
		},
		{
			UID:         "cloudpulse-service-mon",
			Title:       "Service Monitoring",
			Description: "HTTP/TCP service availability, latency, HTTP status codes, and failed checks",
			URL:         fmt.Sprintf("%s/d/cloudpulse-service-mon/service-monitoring%s", c.publicURL, suffix),
		},
		{
			UID:         "cloudpulse-network-mon",
			Title:       "Network Monitoring",
			Description: "Receive/transmit throughput, packet rates, and interface errors",
			URL:         fmt.Sprintf("%s/d/cloudpulse-network-mon/network-monitoring%s", c.publicURL, suffix),
		},
	}
}
