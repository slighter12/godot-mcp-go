package runtimebridge

import "sync"

// NotificationSender pushes a server notification to one MCP session.
type NotificationSender func(sessionID string, message map[string]any) bool

var (
	notificationSenderMu sync.RWMutex
	notificationSender   NotificationSender
)

func SetNotificationSender(sender NotificationSender) {
	notificationSenderMu.Lock()
	defer notificationSenderMu.Unlock()
	notificationSender = sender
}

func sendToSession(sessionID string, message map[string]any) bool {
	notificationSenderMu.RLock()
	sender := notificationSender
	notificationSenderMu.RUnlock()
	if sender == nil {
		return false
	}
	return sender(sessionID, message)
}
