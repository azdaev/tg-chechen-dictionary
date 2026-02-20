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

func (n *Net) HandleBroadcast(ctx context.Context, update *tgbotapi.Update) error {
	if !n.isAdmin(update.Message.From.ID) {
		return nil
	}

	n.awaitingBroadcast = true
	n.pendingBroadcast = nil
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Отправьте текст или фото с подписью. Я покажу превью перед рассылкой.")
	_, err := n.bot.Send(msg)
	return err
}

func (n *Net) HandleBroadcastCancel(update *tgbotapi.Update) error {
	if !n.isAdmin(update.Message.From.ID) {
		return nil
	}

	n.awaitingBroadcast = false
	n.pendingBroadcast = nil
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Рассылка отменена.")
	_, err := n.bot.Send(msg)
	return err
}

func (n *Net) HandleBroadcastContent(update *tgbotapi.Update) error {
	if !n.isAdmin(update.Message.From.ID) {
		return nil
	}

	payload, err := buildBroadcastPayload(update.Message)
	if err != nil {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
		_, sendErr := n.bot.Send(msg)
		return sendErr
	}

	n.awaitingBroadcast = false
	n.pendingBroadcast = payload
	preview, err := n.sendBroadcastPreview(update.Message.Chat.ID, payload)
	if err != nil {
		return err
	}
	_, err = n.bot.Send(preview)
	return err
}

func (n *Net) HandleBroadcastCallback(ctx context.Context, update *tgbotapi.Update) error {
	if update.CallbackQuery == nil || !n.isAdmin(update.CallbackQuery.From.ID) {
		return nil
	}

	switch update.CallbackQuery.Data {
	case "broadcast_send":
		callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Отправляю")
		if _, err := n.bot.Request(callback); err != nil {
			return fmt.Errorf("bot.Request: %w", err)
		}
		go func() {
			if err := n.sendBroadcast(ctx, update); err != nil {
				n.log.WithError(err).Error("service.sendBroadcast")
			}
		}()
		return nil
	case "broadcast_cancel":
		n.pendingBroadcast = nil
		callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Отменено")
		if _, err := n.bot.Request(callback); err != nil {
			return fmt.Errorf("bot.Request: %w", err)
		}
		msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "Рассылка отменена.")
		_, err := n.bot.Send(msg)
		return err
	default:
		return nil
	}
}

func (n *Net) isAwaitingBroadcastContent(update *tgbotapi.Update) bool {
	return update.Message != nil && n.awaitingBroadcast && n.isAdmin(update.Message.From.ID)
}

func (n *Net) sendBroadcast(ctx context.Context, update *tgbotapi.Update) error {
	payload := n.pendingBroadcast
	if payload == nil {
		callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Нет данных для рассылки")
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

	n.pendingBroadcast = nil

	summary := fmt.Sprintf("Рассылка завершена. Всего: %d, отправлено: %d, ошибки: %d, заблокировано: %d", len(userIDs), sent, failed, blocked)
	msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, summary)
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
	if message.Photo != nil && len(message.Photo) > 0 {
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
