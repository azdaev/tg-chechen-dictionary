package net

import (
	"context"
	"errors"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

type noUsersRepo struct {
	Repository
}

func (noUsersRepo) ListUserIDs(context.Context) ([]int64, error) {
	return nil, errors.New("database is locked")
}

// The payload is claimed before the recipient list is read, so a failure there
// used to leave the admin with «Отправляю», nothing sent, and the draft gone —
// visible only as a log line.
func TestSendBroadcast_KeepsTheDraftWhenItNeverStarted(t *testing.T) {
	n := &Net{log: logrus.New(), repo: noUsersRepo{}, bot: &tgbotapi.BotAPI{}}
	payload := &broadcastPayload{Text: "важное объявление"}
	n.setBroadcastState(false, payload)

	cq := &tgbotapi.CallbackQuery{ID: "cb", Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 1}}}
	if err := n.sendBroadcast(context.Background(), cq); err == nil {
		t.Fatal("a broadcast that never started reported success")
	}

	got := n.takePendingBroadcast()
	if got == nil {
		t.Fatal("the draft was dropped; the admin has to retype it")
	}
	if got.Text != payload.Text {
		t.Errorf("draft came back as %q, want %q", got.Text, payload.Text)
	}
}
