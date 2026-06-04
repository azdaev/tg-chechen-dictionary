package net

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type broadcastPayload struct {
	Text     string
	PhotoID  string
	Caption  string
	HasPhoto bool
}

func (n *Net) HandleBroadcast(ctx context.Context, m *tgbotapi.Message) error {
	if !n.isAdmin(m.From.ID) {
		return nil
	}

	n.setBroadcastState(true, nil)
	msg := tgbotapi.NewMessage(m.Chat.ID, "Отправьте текст или фото с подписью. Я покажу превью перед рассылкой.")
	_, err := n.bot.Send(msg)
	return err
}

func (n *Net) HandleBroadcastCancel(m *tgbotapi.Message) error {
	if !n.isAdmin(m.From.ID) {
		return nil
	}

	n.setBroadcastState(false, nil)
	msg := tgbotapi.NewMessage(m.Chat.ID, "Рассылка отменена.")
	_, err := n.bot.Send(msg)
	return err
}

func (n *Net) HandleBroadcastContent(m *tgbotapi.Message) error {
	if !n.isAdmin(m.From.ID) {
		return nil
	}

	payload, err := buildBroadcastPayload(m)
	if err != nil {
		msg := tgbotapi.NewMessage(m.Chat.ID, err.Error())
		_, sendErr := n.bot.Send(msg)
		return sendErr
	}

	n.setBroadcastState(false, payload)
	preview, err := n.sendBroadcastPreview(m.Chat.ID, payload)
	if err != nil {
		return err
	}
	_, err = n.bot.Send(preview)
	return err
}

func (n *Net) HandleBroadcastCallback(ctx context.Context, cq *tgbotapi.CallbackQuery) error {
	if cq == nil || !n.isAdmin(cq.From.ID) {
		return nil
	}

	switch cq.Data {
	case "broadcast_send":
		callback := tgbotapi.NewCallback(cq.ID, "Отправляю")
		if _, err := n.bot.Request(callback); err != nil {
			return fmt.Errorf("bot.Request: %w", err)
		}
		go func() {
			if err := n.sendBroadcast(ctx, cq); err != nil {
				n.log.WithError(err).Error("service.sendBroadcast")
			}
		}()
		return nil
	case "broadcast_cancel":
		n.setBroadcastState(false, nil)
		callback := tgbotapi.NewCallback(cq.ID, "Отменено")
		if _, err := n.bot.Request(callback); err != nil {
			return fmt.Errorf("bot.Request: %w", err)
		}
		msg := tgbotapi.NewMessage(cq.Message.Chat.ID, "Рассылка отменена.")
		_, err := n.bot.Send(msg)
		return err
	default:
		return nil
	}
}

func (n *Net) isAwaitingBroadcastContent(m *tgbotapi.Message) bool {
	if m == nil || !n.isAdmin(m.From.ID) {
		return false
	}
	n.broadcastMu.Lock()
	defer n.broadcastMu.Unlock()
	return n.awaitingBroadcast
}

func (n *Net) setBroadcastState(awaiting bool, payload *broadcastPayload) {
	n.broadcastMu.Lock()
	defer n.broadcastMu.Unlock()
	n.awaitingBroadcast = awaiting
	n.pendingBroadcast = payload
}

// takePendingBroadcast atomically claims the pending payload, so a double tap
// on the send button cannot start two broadcasts.
func (n *Net) takePendingBroadcast() *broadcastPayload {
	n.broadcastMu.Lock()
	defer n.broadcastMu.Unlock()
	p := n.pendingBroadcast
	n.pendingBroadcast = nil
	return p
}

func (n *Net) sendBroadcast(ctx context.Context, cq *tgbotapi.CallbackQuery) error {
	payload := n.takePendingBroadcast()
	if payload == nil {
		callback := tgbotapi.NewCallback(cq.ID, "Нет данных для рассылки")
		_, err := n.bot.Request(callback)
		return err
	}

	userIDs, err := n.repo.ListUserIDs(ctx)
	if err != nil {
		return fmt.Errorf("repo.ListUserIDs: %w", err)
	}

	sent, failed, blocked := 0, 0, 0
	for _, userID := range userIDs {
		if sendErr := n.sendBroadcastPayload(userID, payload); sendErr != nil {
			failed++
			if n.isBlockedError(sendErr) {
				if err := n.repo.MarkUserBlocked(ctx, userID, sendErr.Error()); err != nil {
					n.log.WithError(err).WithField("user_id", userID).Warn("failed to mark user blocked")
				} else {
					blocked++
				}
			}
			n.log.WithError(sendErr).WithField("user_id", userID).Warn("broadcast send failed")
		} else {
			sent++
		}
		time.Sleep(BroadcastSendDelay)
	}

	summary := fmt.Sprintf("Рассылка завершена. Всего: %d, отправлено: %d, ошибки: %d, заблокировано: %d", len(userIDs), sent, failed, blocked)
	msg := tgbotapi.NewMessage(cq.Message.Chat.ID, summary)
	_, err = n.bot.Send(msg)
	return err
}

func (n *Net) sendBroadcastPreview(chatID int64, payload *broadcastPayload) (tgbotapi.Chattable, error) {
	if payload.HasPhoto {
		preview := tgbotapi.NewPhoto(chatID, tgbotapi.FileID(payload.PhotoID))
		preview.Caption = payload.Caption
		preview.ParseMode = BroadcastParseMode
		preview.ReplyMarkup = broadcastPreviewKeyboard()
		return preview, nil
	}

	preview := tgbotapi.NewMessage(chatID, payload.Text)
	preview.ParseMode = BroadcastParseMode
	preview.ReplyMarkup = broadcastPreviewKeyboard()
	return preview, nil
}

func (n *Net) sendBroadcastPayload(chatID int64, payload *broadcastPayload) error {
	if payload.HasPhoto {
		msg := tgbotapi.NewPhoto(chatID, tgbotapi.FileID(payload.PhotoID))
		msg.Caption = payload.Caption
		msg.ParseMode = BroadcastParseMode
		_, err := n.bot.Send(msg)
		return err
	}
	msg := tgbotapi.NewMessage(chatID, payload.Text)
	msg.ParseMode = BroadcastParseMode
	_, err := n.bot.Send(msg)
	return err
}

func buildBroadcastPayload(message *tgbotapi.Message) (*broadcastPayload, error) {
	if message == nil {
		return nil, fmt.Errorf("Нет сообщения для рассылки")
	}
	if len(message.Photo) > 0 {
		photo := message.Photo[len(message.Photo)-1]
		return &broadcastPayload{
			PhotoID:  photo.FileID,
			Caption:  message.Caption,
			HasPhoto: true,
		}, nil
	}
	text := strings.TrimSpace(message.Text)
	if text == "" {
		return nil, fmt.Errorf("Нужен текст или фото с подписью для рассылки")
	}
	return &broadcastPayload{Text: text}, nil
}

func broadcastPreviewKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Отправить", "broadcast_send"),
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "broadcast_cancel"),
		),
	)
}
