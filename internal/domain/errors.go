package domain

import "errors"

// Sentinel errors used throughout the application.
// Handlers translate these to HTTP status codes via a single mapError function.
var (
	ErrNotFound         = errors.New("not found")
	ErrConflict         = errors.New("conflict: idempotency key already exists")
	ErrInvalidChannel   = errors.New("invalid channel: must be sms, email, or push")
	ErrInvalidPriority  = errors.New("invalid priority: must be high, normal, or low")
	ErrInvalidRecipient = errors.New("recipient must not be empty")
	ErrInvalidContent   = errors.New("content must be between 1 and 4096 characters")
	ErrBatchTooLarge    = errors.New("batch exceeds maximum of 1000 notifications")
	ErrBatchEmpty       = errors.New("batch must contain at least one notification")
	ErrAlreadyCancelled = errors.New("notification is already cancelled")
	ErrNotCancellable   = errors.New("notification cannot be cancelled in its current status")
	ErrQueueFull        = errors.New("queue is at capacity, try again later")
)
