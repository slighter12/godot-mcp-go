package runtimebridge

import "sync"

// NotificationSender pushes a server notification to one MCP session.
type NotificationSender func(sessionID string, message map[string]any) bool

var (
	notificationSenderMu sync.RWMutex
	notificationSender   notificationSenderRegistration
	notificationToken    uint64
)

func SetNotificationSender(sender NotificationSender) {
	notificationSenderMu.Lock()
	defer notificationSenderMu.Unlock()
	notificationSender = notificationSenderRegistration{sender: sender}
}

// RegisterNotificationSender installs an owner-scoped sender. Releasing a
// registration only clears the sender if it is still the active registration,
// so an older server cannot tear down a newer server's callback.
func RegisterNotificationSender(sender NotificationSender) func() {
	notificationSenderMu.Lock()
	notificationToken++
	token := notificationToken
	notificationSender = notificationSenderRegistration{token: token, sender: sender}
	notificationSenderMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			notificationSenderMu.Lock()
			defer notificationSenderMu.Unlock()
			if notificationSender.token == token {
				notificationSender = notificationSenderRegistration{}
			}
		})
	}
}

func sendToSession(sessionID string, message map[string]any) bool {
	notificationSenderMu.RLock()
	sender := notificationSender.sender
	notificationSenderMu.RUnlock()
	if sender == nil {
		return false
	}
	return sender(sessionID, message)
}

type notificationSenderRegistration struct {
	token  uint64
	sender NotificationSender
}
