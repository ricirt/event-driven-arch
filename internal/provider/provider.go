package provider

import (
	"context"

	"github.com/notifyhub/event-driven-arch/internal/domain"
)

// SendRequest is the JSON body posted to the external provider.
type SendRequest struct {
	To      string `json:"to"`
	Channel string `json:"channel"`
	Content string `json:"content"`
}

// SendResponse maps the provider's 202 Accepted response body.
type SendResponse struct {
	MessageID string `json:"messageId"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// Provider abstracts delivery to an external notification service.
// Mocking this interface in tests gives full control over provider behaviour
// without making real HTTP calls.
type Provider interface {
	Send(ctx context.Context, n *domain.Notification) (*SendResponse, error)
}
