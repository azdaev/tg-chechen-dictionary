// sim replays what the bot sends for a word, using the live dosham API and the
// bot's own formatter (no DB, no cache).
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"chetoru/internal/models"
	"chetoru/pkg/tools"
)

const maxTranslations = 4

type entry struct {
	Content    string `json:"content"`
	Type       string `json:"type"`
	Rate       int    `json:"rate"`
	Details    string `json:"details"`
	Notes      string `json:"notes"`
	EntryForms []struct {
		Content string `json:"content"`
	} `json:"entryForms"`
	RelatedEntries []struct {
		Content      string `json:"content"`
		Translations []struct {
			Content      string `json:"content"`
			LanguageCode string `json:"languageCode"`
		} `json:"translations"`
	} `json:"relatedEntries"`
	Translations []struct {
		Content      string `json:"content"`
		LanguageCode string `json:"languageCode"`
		Notes        string `json:"notes"`
	} `json:"translations"`
}

func find(word string) []entry {
	q := `query F($t:String!){find(inputText:$t){content type rate details notes entryForms{content} relatedEntries{content translations{content languageCode}} translations{content languageCode notes}}}`
	body, _ := json.Marshal(map[string]any{"query": q, "variables": map[string]string{"t": word}})
	resp, err := http.Post("https://api.dosham.app/gql", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch:", err)
		return nil
	}
	defer resp.Body.Close()
	var payload struct {
		Data struct {
			Find []entry `json:"find"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&payload)
	return payload.Data.Find
}

func normLang(c string) string {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case "ce", "che":
		return "CHE"
	case "ru", "rus":
		return "RUS"
	}
	return ""
}

func main() {
	for _, word := range os.Args[1:] {
		fmt.Printf("\n========== ЗАПРОС: %q ==========\n", word)
		entries := find(word)
		if len(entries) == 0 {
			fmt.Println("(API: 0 записей — бот покажет «К сожалению, нет перевода» + подсказки)")
			continue
		}

		var pairs []models.TranslationPairs
		fmt.Println("--- сырые записи API ---")
		for _, e := range entries {
			for _, t := range e.Translations {
				if normLang(t.LanguageCode) == "" {
					continue
				}
				fmt.Printf("[%s rate=%d forms=%d rel=%d] %s :: %s\n", e.Type, e.Rate, len(e.EntryForms), len(e.RelatedEntries), e.Content, t.Content)
				pairs = append(pairs, models.TranslationPairs{
					Original:  tools.EscapeUnclosedTags(e.Content),
					Translate: tools.EscapeUnclosedTags(t.Content),
				})
			}
		}

		if utf8.RuneCountInString(word) <= 3 && len(pairs) >= 10 {
			pairs = pairs[:10]
		}
		sort.SliceStable(pairs, func(i, j int) bool {
			return utf8.RuneCountInString(pairs[i].Original) < utf8.RuneCountInString(pairs[j].Original)
		})

		shown := pairs
		if len(shown) > maxTranslations {
			shown = shown[:maxTranslations]
		}

		fmt.Println("\n--- СООБЩЕНИЕ БОТА ---")
		fmt.Println(tools.FormatPairs(shown))
		if len(pairs) > maxTranslations {
			fmt.Printf("[кнопка «Еще (%d)»]\n", len(pairs)-maxTranslations)
		}

		// grammar card (mirrors business.computeGrammar / net.formatGrammarCard)
		var best *entry
		for i := range entries {
			e := &entries[i]
			if e.Type != "WORD" {
				continue
			}
			if strings.TrimSpace(e.Details) == "" && len(e.EntryForms) == 0 {
				continue
			}
			if best == nil || e.Rate > best.Rate {
				best = e
			}
		}
		if best != nil {
			fmt.Println("\n--- ГРАММАТИЧЕСКАЯ КАРТОЧКА ---")
			fmt.Printf("📖 %s (details=%s)\n", best.Content, strings.TrimSpace(best.Details))
			if len(best.EntryForms) > 0 {
				var fs []string
				for _, f := range best.EntryForms {
					fs = append(fs, f.Content)
				}
				if len(fs) > 12 {
					fs = fs[:12]
				}
				fmt.Println("Формы: " + strings.Join(fs, ", "))
			}
			n := 0
			for _, r := range best.RelatedEntries {
				var ru string
				for _, t := range r.Translations {
					if normLang(t.LanguageCode) == "RUS" {
						ru = t.Content
						break
					}
				}
				if ru == "" {
					continue
				}
				if n == 0 {
					fmt.Println("\n💬 Выражения:")
				}
				fmt.Printf("• %s — %s\n", r.Content, ru)
				n++
				if n >= 5 {
					break
				}
			}
			fmt.Printf("(всего relatedEntries: %d)\n", len(best.RelatedEntries))
		}
	}
}
