package prometheus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Sample represents a single instant metric sample from Prometheus.
type Sample struct {
	Metric    map[string]string `json:"metric"`
	Value     float64           `json:"value"`
	Timestamp time.Time         `json:"timestamp"`
}

// MatrixSeries represents a time-series result from a Prometheus range query.
type MatrixSeries struct {
	Metric map[string]string `json:"metric"`
	Points []DataPoint       `json:"points"`
}

type DataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// Target represents a scraped target returned by Prometheus /api/v1/targets.
type Target struct {
	Labels            map[string]string `json:"labels"`
	DiscoveredLabels  map[string]string `json:"discoveredLabels"`
	ScrapePool        string            `json:"scrapePool"`
	ScrapeURL         string            `json:"scrapeUrl"`
	LastError         string            `json:"lastError"`
	LastScrape        time.Time         `json:"lastScrape"`
	LastScrapeSeconds float64           `json:"lastScrapeDuration"`
	Health            string            `json:"health"` // up, down, unknown
}

// PromAlert represents a live alert from Prometheus /api/v1/alerts.
type PromAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	State       string            `json:"state"` // firing, pending
	ActiveAt    *time.Time        `json:"activeAt,omitempty"`
	Value       string            `json:"value"`
}

// Client abstracts Prometheus HTTP API interactions for testability.
type Client interface {
	Healthy(ctx context.Context) error
	QueryInstant(ctx context.Context, promQL string, at time.Time) ([]Sample, error)
	QueryRange(ctx context.Context, promQL string, start, end time.Time, step time.Duration) ([]MatrixSeries, error)
	Targets(ctx context.Context) ([]Target, error)
	Alerts(ctx context.Context) ([]PromAlert, error)
	Reload(ctx context.Context) error
}

// HTTPClient implements Client against a real Prometheus server.
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(baseURL string, timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
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
		return fmt.Errorf("prometheus returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *HTTPClient) Reload(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/-/reload", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("prometheus reload failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *HTTPClient) QueryInstant(ctx context.Context, promQL string, at time.Time) ([]Sample, error) {
	params := url.Values{}
	params.Set("query", promQL)
	if !at.IsZero() {
		params.Set("time", strconv.FormatFloat(float64(at.UnixNano())/1e9, 'f', 3, 64))
	}

	var envelope struct {
		Status    string `json:"status"`
		Error     string `json:"error"`
		ErrorType string `json:"errorType"`
		Data      struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []any             `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := c.getJSON(ctx, "/api/v1/query?"+params.Encode(), &envelope); err != nil {
		return nil, err
	}
	if envelope.Status != "success" {
		return nil, fmt.Errorf("prometheus query error (%s): %s", envelope.ErrorType, envelope.Error)
	}

	out := make([]Sample, 0, len(envelope.Data.Result))
	for _, r := range envelope.Data.Result {
		ts, val, ok := parsePromPair(r.Value)
		if !ok {
			continue
		}
		out = append(out, Sample{
			Metric:    r.Metric,
			Timestamp: ts,
			Value:     val,
		})
	}
	return out, nil
}

func (c *HTTPClient) QueryRange(ctx context.Context, promQL string, start, end time.Time, step time.Duration) ([]MatrixSeries, error) {
	if step < 15*time.Second {
		step = 15 * time.Second
	}
	// Cap total points to ~300 per series to protect Prometheus & browser charts.
	if end.Sub(start)/step > 300 {
		step = end.Sub(start) / 300
	}

	params := url.Values{}
	params.Set("query", promQL)
	params.Set("start", strconv.FormatFloat(float64(start.UnixNano())/1e9, 'f', 3, 64))
	params.Set("end", strconv.FormatFloat(float64(end.UnixNano())/1e9, 'f', 3, 64))
	params.Set("step", strconv.Itoa(int(step.Seconds())))

	var envelope struct {
		Status    string `json:"status"`
		Error     string `json:"error"`
		ErrorType string `json:"errorType"`
		Data      struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Values [][]any           `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := c.getJSON(ctx, "/api/v1/query_range?"+params.Encode(), &envelope); err != nil {
		return nil, err
	}
	if envelope.Status != "success" {
		return nil, fmt.Errorf("prometheus range query error (%s): %s", envelope.ErrorType, envelope.Error)
	}

	out := make([]MatrixSeries, 0, len(envelope.Data.Result))
	for _, r := range envelope.Data.Result {
		series := MatrixSeries{
			Metric: r.Metric,
			Points: make([]DataPoint, 0, len(r.Values)),
		}
		for _, pair := range r.Values {
			ts, val, ok := parsePromPair(pair)
			if !ok {
				continue
			}
			series.Points = append(series.Points, DataPoint{Timestamp: ts, Value: val})
		}
		out = append(out, series)
	}
	return out, nil
}

func (c *HTTPClient) Targets(ctx context.Context) ([]Target, error) {
	var envelope struct {
		Status string `json:"status"`
		Data   struct {
			ActiveTargets []Target `json:"activeTargets"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/v1/targets?state=active", &envelope); err != nil {
		return nil, err
	}
	if envelope.Status != "success" {
		return nil, errors.New("prometheus targets query unsuccessful")
	}
	return envelope.Data.ActiveTargets, nil
}

func (c *HTTPClient) Alerts(ctx context.Context) ([]PromAlert, error) {
	var envelope struct {
		Status string `json:"status"`
		Data   struct {
			Alerts []PromAlert `json:"alerts"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/v1/alerts", &envelope); err != nil {
		return nil, err
	}
	if envelope.Status != "success" {
		return nil, errors.New("prometheus alerts query unsuccessful")
	}
	return envelope.Data.Alerts, nil
}

func (c *HTTPClient) getJSON(ctx context.Context, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("prometheus server error (%d)", resp.StatusCode)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("decode prometheus response: %w", err)
	}
	return nil
}

func parsePromPair(pair []any) (time.Time, float64, bool) {
	if len(pair) != 2 {
		return time.Time{}, 0, false
	}
	tsFloat, ok := pair[0].(float64)
	if !ok {
		return time.Time{}, 0, false
	}
	valStr, ok := pair[1].(string)
	if !ok {
		return time.Time{}, 0, false
	}
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return time.Time{}, 0, false
	}
	sec := int64(tsFloat)
	nsec := int64((tsFloat - float64(sec)) * 1e9)
	return time.Unix(sec, nsec).UTC(), val, true
}

// Reusable PromQL query helpers (Section 10.3 — avoid duplicating PromQL across handlers).

var labelSafeRe = regexp.MustCompile(`^[a-zA-Z0-9._:/ -]+$`)

func EscapeLabelValue(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return v
}

func ServerFilter(serverID string) string {
	if strings.TrimSpace(serverID) == "" {
		return ""
	}
	return fmt.Sprintf(`cloudpulse_server_id="%s"`, EscapeLabelValue(serverID))
}

func OrgFilter(orgID string) string {
	if strings.TrimSpace(orgID) == "" {
		return ""
	}
	return fmt.Sprintf(`cloudpulse_org_id="%s"`, EscapeLabelValue(orgID))
}

func CombineSelectors(parts ...string) string {
	var clean []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			clean = append(clean, p)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	return "{" + strings.Join(clean, ",") + "}"
}

func QueryCPUUsage(orgID, serverID string) string {
	return "cloudpulse_cpu_usage_percent" + CombineSelectors(OrgFilter(orgID), ServerFilter(serverID))
}

func QueryCPUCoreUsage(orgID, serverID string) string {
	return "cloudpulse_cpu_core_usage_percent" + CombineSelectors(OrgFilter(orgID), ServerFilter(serverID))
}

func QueryCPUCoreCount(orgID, serverID string) string {
	return "cloudpulse_cpu_core_count" + CombineSelectors(OrgFilter(orgID), ServerFilter(serverID))
}

func QueryCPULoad(orgID, serverID, window string) string {
	return fmt.Sprintf("cloudpulse_cpu_load_%s%s", window, CombineSelectors(OrgFilter(orgID), ServerFilter(serverID)))
}

func QueryMemoryUsage(orgID, serverID string) string {
	return "cloudpulse_memory_usage_percent" + CombineSelectors(OrgFilter(orgID), ServerFilter(serverID))
}

func QueryMemoryMetric(metric, orgID, serverID string) string {
	return metric + CombineSelectors(OrgFilter(orgID), ServerFilter(serverID))
}

func QueryDiskUsage(orgID, serverID string) string {
	return "cloudpulse_disk_usage_percent" + CombineSelectors(OrgFilter(orgID), ServerFilter(serverID))
}

func QueryDiskMaxByServer(orgID string) string {
	return fmt.Sprintf("max by (cloudpulse_server_id) (cloudpulse_disk_usage_percent%s)", CombineSelectors(OrgFilter(orgID)))
}

func QueryDiskMetric(metric, orgID, serverID string) string {
	return metric + CombineSelectors(OrgFilter(orgID), ServerFilter(serverID))
}

func QueryNetworkReceiveRate(orgID, serverID string) string {
	return fmt.Sprintf("rate(cloudpulse_network_receive_bytes_total%s[5m])", CombineSelectors(OrgFilter(orgID), ServerFilter(serverID)))
}

func QueryNetworkTransmitRate(orgID, serverID string) string {
	return fmt.Sprintf("rate(cloudpulse_network_transmit_bytes_total%s[5m])", CombineSelectors(OrgFilter(orgID), ServerFilter(serverID)))
}

func QueryNetworkTotal(metric, orgID, serverID string) string {
	return metric + CombineSelectors(OrgFilter(orgID), ServerFilter(serverID))
}

func QueryAvailability(orgID, serverID string) string {
	return `up` + CombineSelectors(`job="cloudpulse-agents"`, OrgFilter(orgID), ServerFilter(serverID))
}

func QueryServiceUp(orgID, serverID string) string {
	return "cloudpulse_service_up" + CombineSelectors(OrgFilter(orgID), ServerFilter(serverID))
}

func QueryServiceDuration(orgID, serverID string) string {
	return "cloudpulse_service_response_duration_seconds" + CombineSelectors(OrgFilter(orgID), ServerFilter(serverID))
}

func QueryServiceStatusCode(orgID, serverID string) string {
	return "cloudpulse_service_http_status_code" + CombineSelectors(OrgFilter(orgID), ServerFilter(serverID))
}

func QueryServiceTLSExpiry(orgID, serverID string) string {
	return "cloudpulse_service_tls_expiry_seconds" + CombineSelectors(OrgFilter(orgID), ServerFilter(serverID))
}

func QueryAgentBuildInfo(orgID, serverID string) string {
	return "cloudpulse_agent_build_info" + CombineSelectors(OrgFilter(orgID), ServerFilter(serverID))
}

// AllowedMetricPrefixes defines the controlled PromQL metrics exposed via Metrics Explorer.
var AllowedMetricPrefixes = []string{
	"cloudpulse_cpu_",
	"cloudpulse_memory_",
	"cloudpulse_disk_",
	"cloudpulse_network_",
	"cloudpulse_service_",
	"cloudpulse_agent_",
	"up",
}

var disallowedPromQLTokens = []string{
	";", "`", "\n", "\r",
}

// ValidateControlledPromQL restricts browser-submitted queries to safe read-only CloudPulse metrics.
func ValidateControlledPromQL(expr string) error {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return errors.New("query expression is required")
	}
	if len(trimmed) > 512 {
		return errors.New("query expression exceeds 512 character limit")
	}
	for _, bad := range disallowedPromQLTokens {
		if strings.Contains(trimmed, bad) {
			return errors.New("query expression contains disallowed characters")
		}
	}
	// Verify at least one allowed CloudPulse metric identifier appears.
	allowed := false
	for _, prefix := range AllowedMetricPrefixes {
		if strings.Contains(trimmed, prefix) {
			allowed = true
			break
		}
	}
	if !allowed {
		return errors.New("only CloudPulse infrastructure, service, agent, and target availability ('up') metrics are permitted")
	}
	// Check balanced parentheses and braces.
	parens, braces := 0, 0
	for _, ch := range trimmed {
		switch ch {
		case '(':
			parens++
		case ')':
			parens--
		case '{':
			braces++
		case '}':
			braces--
		}
		if parens < 0 || braces < 0 {
			return errors.New("unbalanced parentheses or braces in PromQL expression")
		}
	}
	if parens != 0 || braces != 0 {
		return errors.New("unbalanced parentheses or braces in PromQL expression")
	}
	return nil
}

// ValidateLabelValue ensures user-supplied label filters are safe.
func ValidateLabelValue(v string) bool {
	return v == "" || labelSafeRe.MatchString(v)
}
