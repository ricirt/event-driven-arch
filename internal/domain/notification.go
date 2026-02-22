package domain

import "time"

// Channel is the delivery channel for a notification.
type Channel string

const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
	ChannelPush  Channel = "push"
)

func (c Channel) IsValid() bool {
	switch c {
	case ChannelSMS, ChannelEmail, ChannelPush:
		return true
	}
	return false
}

// Priority controls queue ordering. High is processed first.
type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityNormal Priority = "normal"
	PriorityLow    Priority = "low"
)

func (p Priority) IsValid() bool {
	switch p {
	case PriorityHigh, PriorityNormal, PriorityLow:
		return true
	}
	return false
}

// Status tracks the lifecycle of a notification.
type Status string

const (
	StatusPending    Status = "pending"
	StatusQueued     Status = "queued"
	StatusProcessing Status = "processing"
	StatusSent       Status = "sent"
	StatusFailed     Status = "failed"
	StatusCancelled  Status = "cancelled"
	StatusScheduled  Status = "scheduled"
)

// Notification is the core domain entity.
type Notification struct {
	ID             string     `json:"id"`
	BatchID        *string    `json:"batch_id,omitempty"`
	Channel        Channel    `json:"channel"`
	Recipient      string     `json:"recipient"`
	Content        string     `json:"content"`
	Priority       Priority   `json:"priority"`
	Status         Status     `json:"status"`
	IdempotencyKey *string    `json:"idempotency_key,omitempty"`
	RetryCount     int        `json:"retry_count"`
	MaxRetries     int        `json:"max_retries"`
	NextRetryAt    *time.Time `json:"next_retry_at,omitempty"`
	ScheduledAt    *time.Time `json:"scheduled_at,omitempty"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	ProviderMsgID  *string    `json:"provider_message_id,omitempty"`
	ErrorMessage   *string    `json:"error_message,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Batch groups multiple notifications created together.
type Batch struct {
	ID        string    `json:"id"`
	Total     int       `json:"total"`
	Pending   int       `json:"pending"`
	Sent      int       `json:"sent"`
	Failed    int       `json:"failed"`
	Cancelled int       `json:"cancelled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateNotificationRequest is the inbound payload for a single notification.
type CreateNotificationRequest struct {
	Channel     Channel    `json:"channel"`
	Recipient   string     `json:"recipient"`
	Content     string     `json:"content"`
	Priority    Priority   `json:"priority"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}

func (r *CreateNotificationRequest) Validate() error {
	if !r.Channel.IsValid() {
		return ErrInvalidChannel
	}
	if !r.Priority.IsValid() {
		return ErrInvalidPriority
	}
	if r.Recipient == "" {
		return ErrInvalidRecipient
	}
	if r.Content == "" || len(r.Content) > 4096 {
		return ErrInvalidContent
	}
	return nil
}

// CreateBatchRequest wraps a slice of notification requests.
type CreateBatchRequest struct {
	Notifications []CreateNotificationRequest `json:"notifications"`
}

// ListFilter holds query parameters for paginated notification listing.
type ListFilter struct {
	Status  *Status
	Channel *Channel
	From    *time.Time
	To      *time.Time
	Page    int
	Limit   int
}
