package net

import (
	"context"
	"os"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

// callback_data is client-supplied: a custom client can send any string, and
// every /quiz or /random card gives it a message to attach to. Before the guard
// the handler wrote straight to the dictionary, so «mod_delete_<id>» walked over
// the integer ids emptied the local table for every user.
func TestModerationCallback_RejectsPressesOutsideTheModerationChat(t *testing.T) {
	t.Setenv("TG_MOD_CHAT_ID", "-1001234567890")
	modChat, err := strconvParseInt(os.Getenv("TG_MOD_CHAT_ID"))
	if err != nil {
		t.Fatalf("test setup: %v", err)
	}

	repo := &authProbeRepo{}
	n := &Net{log: logrus.New(), repo: repo}

	// A stranger's chat: the write must not happen.
	outsider := &tgbotapi.CallbackQuery{
		Data:    "mod_delete_1",
		From:    &tgbotapi.User{ID: 999},
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 555}},
	}
	if err := n.HandleModerationCallback(context.Background(), outsider); err != nil {
		t.Fatalf("outsider press returned an error instead of being ignored: %v", err)
	}
	if repo.calls != 0 {
		t.Fatalf("a forged callback reached the dictionary: %d writes", repo.calls)
	}

	// A callback with no message at all must not panic or write.
	if err := n.HandleModerationCallback(context.Background(), &tgbotapi.CallbackQuery{
		Data: "mod_delete_1", From: &tgbotapi.User{ID: 999},
	}); err != nil {
		t.Fatalf("message-less callback returned an error: %v", err)
	}
	if repo.calls != 0 {
		t.Fatalf("a message-less callback reached the dictionary: %d writes", repo.calls)
	}

	// From the moderation chat it still works — the whole chat moderates.
	// The handler edits the message afterwards, which needs a live bot this
	// test does not have; the write is what is under test, so the send panic is
	// swallowed rather than mocked.
	func() {
		defer func() { _ = recover() }()
		_ = n.HandleModerationCallback(context.Background(), &tgbotapi.CallbackQuery{
			Data:    "mod_delete_1",
			From:    &tgbotapi.User{ID: 999},
			Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: modChat}},
		})
	}()
	if repo.calls != 1 {
		t.Fatalf("the moderation chat lost its buttons: %d writes, want 1", repo.calls)
	}
}
