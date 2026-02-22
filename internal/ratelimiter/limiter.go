package ratelimiter

import (
	"context"

	"golang.org/x/time/rate"

	"github.com/notifyhub/event-driven-arch/internal/domain"
)

// ChannelLimiters holds one token bucket limiter per channel type.
// Each limiter enforces a steady-state rate (e.g. 100 tokens/sec).
// Burst is set equal to the rate so no extra burst capacity is allowed
// beyond the configured per-second maximum.
type ChannelLimiters struct {
	limiters map[domain.Channel]*rate.Limiter
}

// New creates a ChannelLimiters with ratePerSec tokens per second per channel.
func New(ratePerSec int) *ChannelLimiters {
	r := rate.Limit(ratePerSec)
	burst := ratePerSec // burst == rate: prevents any "saved up" burst above the limit

	return &ChannelLimiters{
		limiters: map[domain.Channel]*rate.Limiter{
			domain.ChannelSMS:   rate.NewLimiter(r, burst),
			domain.ChannelEmail: rate.NewLimiter(r, burst),
			domain.ChannelPush:  rate.NewLimiter(r, burst),
		},
	}
}

// Wait blocks until the channel's limiter grants a token.
// Called by each worker immediately before sending to the provider.
// Returns a non-nil error only if ctx is cancelled while waiting.
func (cl *ChannelLimiters) Wait(ctx context.Context, ch domain.Channel) error {
	return cl.limiters[ch].Wait(ctx)
}
