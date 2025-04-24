// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

func (pm *PushNotificationManager) handleSystemEvent(event *model.Event) []*pn.Notification[pn.SubscriptionTopic] {
	notifications := make([]*pn.Notification[pn.SubscriptionTopic], 0)

	for _, language := range getAvailableLanguages() {
		notifications = append(notifications, &pn.Notification[pn.SubscriptionTopic]{
			Target: pn.SubscriptionTopic("system_" + language),
			Data: map[string]interface{}{
				"event": event.String(),
			},
		})
	}

	return notifications
}

func getAvailableLanguages() []string {
	return []string{"en", "zh", "es", "fr", "de", "ru"}
}
