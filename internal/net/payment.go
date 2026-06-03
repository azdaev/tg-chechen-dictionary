package net

import (
	"context"
	"fmt"
	"os"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// canUseSpellcheck checks if the user has free uses left or an active subscription.
// Returns true if allowed, false if paywall should be shown.
func (n *Net) canUseSpellcheck(ctx context.Context, userID int64) (bool, error) {
	// Check subscription first
	hasSub, err := n.repo.HasActiveSubscription(ctx, userID)
	if err != nil {
		return false, err
	}
	if hasSub {
		return true, nil
	}

	// Check free tier
	now := time.Now()
	usage, err := n.repo.GetSpellcheckUsage(ctx, userID, int(now.Month()), now.Year())
	if err != nil {
		return false, err
	}

	return usage < FreeSpellcheckLimit, nil
}

// trackSpellcheckUsage increments the usage counter for the current month.
func (n *Net) trackSpellcheckUsage(ctx context.Context, userID int64) {
	now := time.Now()
	if err := n.repo.IncrementSpellcheckUsage(ctx, userID, int(now.Month()), now.Year()); err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("failed to track spellcheck usage")
	}
}

// sendInvoice sends a Telegram Payments invoice for the spellcheck subscription.
func (n *Net) sendInvoice(chatID int64) error {
	providerToken := os.Getenv("PAYMENT_PROVIDER_TOKEN")

	invoice := tgbotapi.InvoiceConfig{
		BaseChat: tgbotapi.BaseChat{ChatID: chatID},
		Title:    "Проверка орфографии — подписка",
		Description: fmt.Sprintf(
			"Безлимитная проверка орфографии чеченского языка на 30 дней. Бесплатно: %d проверок/мес.",
			FreeSpellcheckLimit,
		),
		Payload:       "spellcheck_subscription",
		ProviderToken: providerToken,
		Currency:      "RUB",
		Prices: []tgbotapi.LabeledPrice{
			{Label: "Подписка (30 дней)", Amount: SubscriptionPriceKopecks},
		},
		SuggestedTipAmounts: []int{},
	}

	_, err := n.bot.Send(invoice)
	return err
}

// HandleSubscribe shows subscription status or sends an invoice.
func (n *Net) HandleSubscribe(ctx context.Context, m *tgbotapi.Message) error {
	userID := m.From.ID

	hasSub, err := n.repo.HasActiveSubscription(ctx, userID)
	if err != nil {
		return fmt.Errorf("repo.HasActiveSubscription: %w", err)
	}

	if hasSub {
		msg := tgbotapi.NewMessage(m.Chat.ID, "✅ У вас уже есть активная подписка. Проверка орфографии без ограничений!")
		_, err = n.bot.Send(msg)
		return err
	}

	// Show remaining free uses
	now := time.Now()
	usage, _ := n.repo.GetSpellcheckUsage(ctx, userID, int(now.Month()), now.Year())
	remaining := FreeSpellcheckLimit - usage
	if remaining < 0 {
		remaining = 0
	}

	providerToken := os.Getenv("PAYMENT_PROVIDER_TOKEN")

	var text string
	if providerToken != "" {
		text = fmt.Sprintf(
			"📝 Проверка орфографии чеченского языка\n\n"+
				"В боте (/check, .текст) — бесплатно и без ограничений\n\n"+
				"Инлайн-режим (@chetoru_bot . текст):\n"+
				"• Бесплатно: %d проверок/мес (осталось: %d)\n"+
				"• Подписка: %s/мес — безлимит\n\n"+
				"Нажмите кнопку ниже для оплаты:",
			FreeSpellcheckLimit, remaining, SubscriptionPriceFormatted,
		)
	} else {
		text = fmt.Sprintf(
			"📝 Проверка орфографии чеченского языка\n\n"+
				"В боте (/check, .текст) — бесплатно и без ограничений\n\n"+
				"Инлайн-режим (@chetoru_bot . текст):\n"+
				"• Бесплатно: %d проверок/мес (осталось: %d)\n"+
				"• Подписка: %s/мес — безлимит\n\n"+
				"⚠️ Оплата пока не подключена. Обратитесь к @azdaev.",
			FreeSpellcheckLimit, remaining, SubscriptionPriceFormatted,
		)
	}

	infoMsg := tgbotapi.NewMessage(m.Chat.ID, text)
	if _, err = n.bot.Send(infoMsg); err != nil {
		return err
	}

	if providerToken != "" {
		return n.sendInvoice(m.Chat.ID)
	}
	return nil
}

// HandlePreCheckout approves pre-checkout queries from Telegram Payments.
func (n *Net) HandlePreCheckout(pq *tgbotapi.PreCheckoutQuery) error {
	// Validate payload
	if pq.InvoicePayload != "spellcheck_subscription" {
		answer := tgbotapi.PreCheckoutConfig{
			PreCheckoutQueryID: pq.ID,
			OK:                 false,
			ErrorMessage:       "Неизвестный тип платежа",
		}
		_, err := n.bot.Request(answer)
		return err
	}

	answer := tgbotapi.PreCheckoutConfig{
		PreCheckoutQueryID: pq.ID,
		OK:                 true,
	}
	_, err := n.bot.Request(answer)
	return err
}

// HandleSuccessfulPayment processes successful payments and activates subscriptions.
func (n *Net) HandleSuccessfulPayment(ctx context.Context, m *tgbotapi.Message) error {
	payment := m.SuccessfulPayment
	userID := m.From.ID

	if payment.InvoicePayload != "spellcheck_subscription" {
		return nil
	}

	expiresAt := time.Now().Add(SubscriptionDuration)

	if err := n.repo.CreateSubscription(ctx, userID, expiresAt, payment.TelegramPaymentChargeID); err != nil {
		n.log.WithError(err).WithField("user_id", userID).Error("failed to create subscription")
		msg := tgbotapi.NewMessage(m.Chat.ID, "⚠️ Оплата прошла, но произошла ошибка. Обратитесь к администратору.")
		n.bot.Send(msg)
		return err
	}

	msg := tgbotapi.NewMessage(m.Chat.ID, fmt.Sprintf(
		"✅ Подписка активирована!\n\nБезлимитная проверка орфографии до %s.\n\nИспользуйте /check или начните сообщение с точки.",
		expiresAt.Format("02.01.2006"),
	))
	_, err := n.bot.Send(msg)
	return err
}
