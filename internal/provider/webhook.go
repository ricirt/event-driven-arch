package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/notifyhub/event-driven-arch/internal/domain"
)

// WebhookProvider delivers notifications by POSTing to webhook.site.
// The base URL is injected from config so tests can point to a local mock.
type WebhookProvider struct {
	baseURL    string
	httpClient *http.Client
}

func NewWebhookProvider(baseURL string, timeout time.Duration) *WebhookProvider {
	return &WebhookProvider{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Send posts the notification to the configured webhook URL and
// expects a 202 Accepted response with a JSON body containing messageId.
func (p *WebhookProvider) Send(ctx context.Context, n *domain.Notification) (*SendResponse, error) {
	body, err := json.Marshal(SendRequest{
		To:      n.Recipient,
		Channel: string(n.Channel),
		Content: n.Content,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("unexpected provider status: %d", resp.StatusCode)
	}

	var sendResp SendResponse
	if err := json.NewDecoder(resp.Body).Decode(&sendResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &sendResp, nil
}

// compile-time check that WebhookProvider implements Provider
var _ Provider = (*WebhookProvider)(nil)
