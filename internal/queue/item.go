package queue

import "github.com/ricirt/event-driven-arch/internal/domain"

// Item is the minimal data placed on the queue.
// Workers fetch the full Notification from the DB using the ID,
// keeping the queue lightweight and the domain data authoritative.
type Item struct {
	NotificationID string
	Channel        domain.Channel
	Priority       domain.Priority
}
