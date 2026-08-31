package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// sendRich posts one card to a chat so a human can look at it. There is no
// other way to see how Telegram renders rich markup: the API validates the
// chat before the HTML, so a probe against a nonexistent chat answers «chat not
// found» whatever the markup says.
//
// Deliberately manual and deliberately loud. The token in .env belongs to the
// production bot, so nothing here runs unless a chat id is passed by hand:
//
//	go run ./cmd/sim -send <chat_id> собака къолам
func sendRich(chatID, html string) {
	token := os.Getenv("TG_BOT_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "нет TG_BOT_TOKEN в окружении")
		return
	}
	body, _ := json.Marshal(map[string]any{
		"chat_id":      chatID,
		"rich_message": map[string]string{"html": html},
	})
	resp, err := http.Post(
		"https://api.telegram.org/bot"+token+"/sendRichMessage",
		"application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(os.Stderr, "отправка:", err)
		return
	}
	defer resp.Body.Close()

	var out struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if !out.OK {
		fmt.Printf("  ✗ Telegram отказал: %s\n", out.Description)
		return
	}
	// The returned Message is what Telegram made of our HTML — the one place a
	// silently dropped tag would show up without opening the app.
	fmt.Printf("  ✓ отправлено, Telegram разобрал так:\n%s\n", indent(out.Result))
}

func indent(raw json.RawMessage) string {
	var buf bytes.Buffer
	if json.Indent(&buf, raw, "    ", "  ") != nil {
		return "    " + string(raw)
	}
	return "    " + buf.String()
}
