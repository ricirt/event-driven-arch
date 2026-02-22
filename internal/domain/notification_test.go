package domain_test

import (
	"strings"
	"testing"

	"github.com/ricirt/event-driven-arch/internal/domain"
)

func TestCreateNotificationRequest_Validate(t *testing.T) {
	valid := domain.CreateNotificationRequest{
		Channel:   domain.ChannelSMS,
		Recipient: "+905551234567",
		Content:   "Hello",
		Priority:  domain.PriorityNormal,
	}

	t.Run("valid request passes", func(t *testing.T) {
		if err := valid.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("invalid channel", func(t *testing.T) {
		r := valid
		r.Channel = "fax"
		if err := r.Validate(); err != domain.ErrInvalidChannel {
			t.Fatalf("expected ErrInvalidChannel, got %v", err)
		}
	})

	t.Run("invalid priority", func(t *testing.T) {
		r := valid
		r.Priority = "urgent"
		if err := r.Validate(); err != domain.ErrInvalidPriority {
			t.Fatalf("expected ErrInvalidPriority, got %v", err)
		}
	})

	t.Run("empty recipient", func(t *testing.T) {
		r := valid
		r.Recipient = ""
		if err := r.Validate(); err != domain.ErrInvalidRecipient {
			t.Fatalf("expected ErrInvalidRecipient, got %v", err)
		}
	})

	t.Run("empty content", func(t *testing.T) {
		r := valid
		r.Content = ""
		if err := r.Validate(); err != domain.ErrInvalidContent {
			t.Fatalf("expected ErrInvalidContent, got %v", err)
		}
	})

	t.Run("content too long", func(t *testing.T) {
		r := valid
		r.Content = strings.Repeat("x", 4097)
		if err := r.Validate(); err != domain.ErrInvalidContent {
			t.Fatalf("expected ErrInvalidContent, got %v", err)
		}
	})

	t.Run("content at max length passes", func(t *testing.T) {
		r := valid
		r.Content = strings.Repeat("x", 4096)
		if err := r.Validate(); err != nil {
			t.Fatalf("expected no error at max length, got %v", err)
		}
	})

	t.Run("all valid channels accepted", func(t *testing.T) {
		for _, ch := range []domain.Channel{domain.ChannelSMS, domain.ChannelEmail, domain.ChannelPush} {
			r := valid
			r.Channel = ch
			if err := r.Validate(); err != nil {
				t.Fatalf("channel %q: expected no error, got %v", ch, err)
			}
		}
	})

	t.Run("all valid priorities accepted", func(t *testing.T) {
		for _, p := range []domain.Priority{domain.PriorityHigh, domain.PriorityNormal, domain.PriorityLow} {
			r := valid
			r.Priority = p
			if err := r.Validate(); err != nil {
				t.Fatalf("priority %q: expected no error, got %v", p, err)
			}
		}
	})
}
