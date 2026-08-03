// Command parse_articles breaks the Russian–Chechen article corpus into
// structured senses and examples, once, offline.
//
// The bot renders from dosham's own fields for three of its four source
// dictionaries. The fourth stores a whole entry as one string, and the regex
// parser in pkg/tools cannot always tell a gloss from an example there, nor
// expand a tilde under a stem Russian spelling does not determine. That is a
// job for a model — but at ingest, writing data, never at render writing
// layout, which is what made the retired AI card formatter produce a second
// competing format.
//
//	go run ./cmd/parse_articles -db bot.db [-limit 500] [-workers 8] [-dry]
package main

import (
	"chetoru/internal/ai"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	_ "modernc.org/sqlite"
)

type row struct {
	id       int64
	headword string
	article  string
}

func main() {
	dbPath := flag.String("db", "bot.db", "path to the SQLite database")
	limit := flag.Int("limit", 500, "how many articles to process this run")
	workers := flag.Int("workers", 8, "concurrent model calls")
	dry := flag.Bool("dry", false, "print results instead of writing them")
	flag.Parse()

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("AI_MODEL")
	if model == "" {
		model = "anthropic/claude-haiku-4.5"
	}
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "OPENROUTER_API_KEY is not set")
		os.Exit(1)
	}

	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	client := ai.New(apiKey, model, log)

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}
	defer db.Close()

	rows, err := pending(db, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "select:", err)
		os.Exit(1)
	}
	fmt.Printf("статей к разбору: %d (модель %s)\n", len(rows), model)
	if len(rows) == 0 {
		return
	}

	var (
		mu             sync.Mutex
		done, failed   int
		work           = make(chan row)
		wg             sync.WaitGroup
		started        = time.Now()
		writeErrShown  bool
		reportInterval = 25
	)

	for range *workers {
		wg.Go(func() {
			for r := range work {
				// A per-article timeout keeps one hung call from stalling the run;
				// a failure just leaves the row for the next pass.
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				structure, err := client.StructureArticle(ctx, r.headword, r.article)
				cancel()

				mu.Lock()
				if err != nil {
					failed++
					log.WithError(err).WithField("word", r.headword).Warn("разбор не удался")
					mu.Unlock()
					continue
				}
				done++
				if done%reportInterval == 0 {
					fmt.Printf("  %d готово, %d ошибок, %s\n", done, failed, time.Since(started).Round(time.Second))
				}
				mu.Unlock()

				payload, err := json.Marshal(structure)
				if err != nil {
					continue
				}
				if *dry {
					mu.Lock()
					fmt.Printf("\n%s\n  %s\n  → %s\n", r.headword, r.article, payload)
					mu.Unlock()
					continue
				}
				if _, err := db.Exec(
					`update dictionary_pairs set structured_json = ? where id = ?;`,
					string(payload), r.id,
				); err != nil && !writeErrShown {
					mu.Lock()
					writeErrShown = true
					log.WithError(err).Error("запись не удалась")
					mu.Unlock()
				}
			}
		})
	}

	for _, r := range rows {
		work <- r
	}
	close(work)
	wg.Wait()

	fmt.Printf("готово: %d разобрано, %d ошибок, %s\n", done, failed, time.Since(started).Round(time.Second))
}

// pending returns unparsed articles: rows whose translation is the Chechen
// side, which is exactly the Russian–Chechen corpus and nothing else.
func pending(db *sql.DB, limit int) ([]row, error) {
	rs, err := db.Query(
		`select id, original_raw, translation_raw
		 from dictionary_pairs
		 where translation_lang = 'CHE'
		   and structured_json is null
		   and (formatted_chosen is null or formatted_chosen != 'deleted')
		 limit ?;`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rs.Close()

	var out []row
	for rs.Next() {
		var r row
		if err := rs.Scan(&r.id, &r.headword, &r.article); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rs.Err()
}
