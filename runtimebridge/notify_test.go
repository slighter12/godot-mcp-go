package runtimebridge

import "testing"

func TestRegisterNotificationSenderReleaseDoesNotClearNewerOwner(t *testing.T) {
	SetNotificationSender(nil)
	defer SetNotificationSender(nil)

	oldRelease := RegisterNotificationSender(func(_ string, _ map[string]any) bool {
		return false
	})
	newRelease := RegisterNotificationSender(func(_ string, _ map[string]any) bool {
		return true
	})

	oldRelease()
	if !sendToSession("editor-1", map[string]any{"method": "test"}) {
		t.Fatal("expected newer notification sender to remain active")
	}

	newRelease()
	if sendToSession("editor-1", map[string]any{"method": "test"}) {
		t.Fatal("expected released notification sender to be inactive")
	}
}
