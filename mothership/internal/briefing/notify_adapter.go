// Package briefing provides notification adapter for the briefing scheduler.
package briefing

import (
	"log"

	"github.com/spaxel/mothership/internal/notify"
)

// NotifyAdapter adapts the notify.Service to the briefing NotifyService interface.
type NotifyAdapter struct {
	service *notify.Service
}

// NewNotifyAdapter creates a new notification adapter.
func NewNotifyAdapter(svc *notify.Service) *NotifyAdapter {
	return &NotifyAdapter{service: svc}
}

// Send sends a notification through the notify service.
func (a *NotifyAdapter) Send(notification Notification) error {
	if a.service == nil {
		log.Printf("[WARN] Notification service not available, skipping: %s", notification.Title)
		return nil
	}

	notif := notify.Notification{
		Title:     notification.Title,
		Body:      notification.Body,
		Priority:  notification.Priority,
		Tags:      notification.Tags,
		Image:     notification.Image,
		ImageType: notification.ImageType,
		Timestamp: notification.Timestamp,
	}

	return a.service.Send(notif)
}
