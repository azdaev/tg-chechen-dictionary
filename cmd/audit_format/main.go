// Command audit_format samples random entries from the live dosham API,
// renders each gloss through the same formatter the bot uses, and flags
// renders that leak dictionary metadata (markers, raw tildes, sense numbers,
// brackets). Run it before and after formatter changes:
//
//	go run ./cmd/audit_format -n 200
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"chetoru/pkg/tools"
)

type entry struct {
	Content      string `json:"content"`
	Type         string `json:"type"`
	Translations []struct {
		Content      string `json:"content"`
		LanguageCode string `json:"languageCode"`
	} `json:"translations"`
}

var (
	senseLeftover  = regexp.MustCompile(`\d\)`)
	markerLeftover = regexp.MustCompile(`(^|\n)[^—\n]{0,2}— ?([а-яёӏ] |-[а-яё]{1,3}[,;]|нескл\.|(не)?сов\.)`)
)

func suspicions(rendered string) []string {
	var out []string
	if strings.Contains(rendered, "~") {
		out = append(out, "raw tilde")
	}
	if senseLeftover.MatchString(rendered) {
		out = append(out, "sense number leftover")
	}
	if strings.ContainsAny(rendered, "<>") {
		out = append(out, "angle bracket")
	}
	if markerLeftover.MatchString(rendered) {
		out = append(out, "marker in header")
	}
	if first, _, _ := strings.Cut(rendered, "\n"); strings.HasSuffix(strings.TrimSpace(first), "—") {
		out = append(out, "empty header translation")
	}
	return out
}

func main() {
	n := flag.Int("n", 100, "entries to sample")
	examples := flag.Bool("examples", false, "print extracted usage examples instead of flagging renders")
	flag.Parse()

	apiURL := os.Getenv("DOSHAM_API_URL")
	if apiURL == "" {
		apiURL = "https://api.dosham.app/gql"
	}

	query := fmt.Sprintf(`{ randomEntries(count: %d) { content type translations { content languageCode } } }`, *n)
	body, _ := json.Marshal(map[string]string{"query": query})
	resp, err := http.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var payload struct {
		Data struct {
			RandomEntries []entry `json:"randomEntries"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}

	rendered, flagged := 0, 0
	for _, e := range payload.Data.RandomEntries {
		if e.Type != "WORD" {
			continue
		}
		for _, t := range e.Translations {
			lang := strings.ToLower(t.LanguageCode)
			if lang != "ce" && lang != "che" {
				continue
			}
			rendered++
			// Only CE translations get here, so the gloss is the Chechen side
			// and the headword is Russian.
			if *examples {
				if ex, ok := tools.FirstExample(t.Content, e.Content, false); ok {
					flagged++
					fmt.Printf("%s: %s\n", e.Content, ex)
				}
				continue
			}
			out := tools.FormatTranslationLite("**"+e.Content+"** - "+t.Content, e.Content, true)
			if sus := suspicions(out); len(sus) > 0 {
				flagged++
				fmt.Printf("=== %s [%s]\nGLOSS: %s\nOUT:\n%s\n\n", e.Content, strings.Join(sus, ", "), t.Content, out)
			}
		}
	}
	if *examples {
		fmt.Printf("rendered %d glosses, %d carry an example\n", rendered, flagged)
		return
	}
	fmt.Printf("rendered %d glosses, flagged %d\n", rendered, flagged)
}
