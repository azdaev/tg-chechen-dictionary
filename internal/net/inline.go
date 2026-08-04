// Inline mode: the picker shown when the bot is typed into another chat.
package net

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (n *Net) HandleInline(ctx context.Context, iq *tgbotapi.InlineQuery) error {
	// An empty query is the "@bot " moment in someone's chat — serve a few
	// discovery words instead of a blank screen.
	if strings.TrimSpace(iq.Query) == "" {
		return n.answerInlineDiscovery(ctx, iq)
	}

	translations, resolved, err := n.business.TranslateResolved(iq.Query)
	if err != nil {
		n.log.WithError(err).WithField("query", iq.Query).Warn("inline lookup failed")
		return n.answerInlineUnavailable(iq)
	}
	// The card renders against the headword that answered, not against what was
	// typed: «лоьман» is resolved to «лом», and a card keyed on the form matches
	// none of the lemma's pairs.
	renderKey := iq.Query
	if resolved != "" {
		renderKey = resolved
	}

	articles := inlineArticles(iq.ID, renderKey, translations, false)

	// A dead-end inline query used to show nothing at all; rescue it the same
	// way the text path does — with lemma suggestions for the typed prefix.
	if len(articles) == 0 {
		articles = inlineArticles(iq.ID, iq.Query, n.business.SuggestTranslations(iq.Query), true)
	}

	// Dictionary results are identical for everyone and effectively static, so
	// let Telegram cache them on its edge: repeated queries (every keystroke
	// counts as one) are then answered without reaching the bot at all. The
	// spellcheck inline path stays personal — it has per-user quotas.
	inlineConf := tgbotapi.InlineConfig{
		InlineQueryID: iq.ID,
		IsPersonal:    false,
		CacheTime:     InlineCacheTimeSeconds,
		Results:       articles,
	}

	if err := n.answerInline(inlineConf); err != nil {
		return fmt.Errorf("answerInline: %w", err)
	}

	n.recordActivity(ctx, iq.From.ID, iq.From.UserName, models.ActivityTypeInline)
	return nil
}

// inlineArticles builds the picker's rows. The first offers the whole card —
// every sense, the direction chip, the examples — and the rest offer one entry
// each, for when only one is wanted.
//
// Returning nothing when that card is empty is the point: whether a lookup
// succeeded is one question with one answer. The picker used to ask a different
// one — "did dosham return any rows?" — so a query that matched nothing but
// noise was «нет перевода» in a chat and a list of results in the picker.
func inlineArticles(id, query string, pairs []models.TranslationPairs, suggested bool) []any {
	// Telegram allows at most 50 results per inline query; sending more makes
	// answerInlineQuery fail and the user sees nothing. Cap defensively — common
	// words (e.g. "дать") can have far more than 50 translation pairs.
	if len(pairs) > InlineResultsLimit-1 {
		pairs = pairs[:InlineResultsLimit-1]
	}

	articles := make([]any, 0, len(pairs)+1)
	if !suggested {
		card := clampMessage(tools.FormatCard(query, pairs))
		if card == "" {
			return nil
		}
		articles = append(articles, inlineArticle(id+"card", tools.Clean(query), inlineDescription(tools.Summary(query, pairs)), card))
	}

	for i := range pairs {
		title := tools.Clean(pairs[i].Original)
		if strings.TrimSpace(title) == "" {
			// Telegram rejects the entire answer if any article title is empty,
			// so one malformed entry would blank out the whole inline response.
			continue
		}
		if suggested {
			title = "🔍 " + title
		}
		// The sent message gets the same card the text path produces, instead
		// of dumping the raw gloss ("м 1) цӏа; деревянный ~- …").
		formatted := clampMessage(tools.FormatCard(pairs[i].Original, pairs[i:i+1]))
		if formatted == "" {
			// Telegram rejects empty message content, and a collocation renders
			// no card of its own — it is an example, not an entry.
			formatted = clampMessage(tools.FormatPairs(pairs[i : i+1]))
		}
		if formatted == "" {
			continue
		}
		// The subtitle says what the entry means, in the words the card uses.
		// Never by slicing the card itself: the picker shows plain text, so a
		// <b> would reach the reader as literal tags, and cutting a headword
		// prefix off the front breaks the moment the card's first line changes.
		desc := tools.Summary(pairs[i].Original, pairs[i:i+1])
		if desc == "" {
			desc = pairs[i].Translate // a collocation has no card of its own
		}
		articles = append(articles, inlineArticle(id+strconv.Itoa(i), title, inlineDescription(desc), formatted))
	}
	return articles
}

func inlineArticle(id, title, description, text string) tgbotapi.InlineQueryResultArticle {
	article := tgbotapi.NewInlineQueryResultArticle(id, title, "")
	article.Description = description
	article.InputMessageContent = tgbotapi.InputTextMessageContent{Text: text, ParseMode: "html"}
	return article
}

// answerInlineUnavailable tells the user the dictionary is down instead of
// leaving a dead picker with no explanation. CacheTime is zero and the answer
// is personal, so Telegram's edge does not keep an outage on screen after it
// ends — the reason the failure path used to answer nothing at all.
func (n *Net) answerInlineUnavailable(iq *tgbotapi.InlineQuery) error {
	return n.answerInline(inlineUnavailableConfig(iq.ID))
}

func inlineUnavailableConfig(queryID string) tgbotapi.InlineConfig {
	article := tgbotapi.NewInlineQueryResultArticle(queryID+"_down", "⚠️ Словарь недоступен", "")
	article.Description = "Попробуйте через минуту"
	article.InputMessageContent = tgbotapi.InputTextMessageContent{Text: DictionaryUnavailableText}
	return tgbotapi.InlineConfig{
		InlineQueryID: queryID,
		IsPersonal:    true,
		CacheTime:     0,
		Results:       []any{article},
	}
}

// answerInlineDiscovery responds to an empty inline query with a few random
// dictionary words. Pool-backed, so the common case costs no API call.
func (n *Net) answerInlineDiscovery(ctx context.Context, iq *tgbotapi.InlineQuery) error {
	articles := make([]any, 0, InlineDiscoveryCount)
	for i := range InlineDiscoveryCount {
		w, err := n.business.RandomWordFromAPI(ctx)
		if err != nil || w == nil {
			break
		}
		text := fmt.Sprintf(
			RandomWordFormat,
			tgbotapi.EscapeText(tgbotapi.ModeHTML, w.Chechen),
			tgbotapi.EscapeText(tgbotapi.ModeHTML, w.Russian),
		)
		article := tgbotapi.NewInlineQueryResultArticle(iq.ID+strconv.Itoa(i), "🎲 "+w.Chechen, "")
		article.Description = w.Russian
		article.InputMessageContent = tgbotapi.InputTextMessageContent{
			Text:      text,
			ParseMode: "html",
		}
		articles = append(articles, article)
	}
	if len(articles) == 0 {
		return nil // pool empty and API down; leave the placeholder screen
	}

	inlineConf := tgbotapi.InlineConfig{
		InlineQueryID: iq.ID,
		IsPersonal:    false,
		CacheTime:     InlineDiscoveryCacheSec,
		Results:       articles,
	}
	if err := n.answerInline(inlineConf); err != nil {
		return fmt.Errorf("answerInline: %w", err)
	}
	return nil
}

// inlineDescription renders the one-line subtitle under an inline result. It
// takes plain text — a gloss or tools.Summary — never the rendered card, whose
// markup Telegram would show literally.
func inlineDescription(gloss string) string {
	desc := strings.Join(strings.Fields(tools.Clean(gloss)), " ")
	runes := []rune(desc)
	if len(runes) <= inlineDescriptionRunes {
		return desc
	}
	cut := string(runes[:inlineDescriptionRunes])
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return cut + "…"
}

// inlineDescriptionRunes caps the inline picker's subtitle. Telegram truncates
// longer ones itself, but a gloss cut mid-word by the client reads worse than
// one cut at a boundary here.
const inlineDescriptionRunes = 100
