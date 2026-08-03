package business

import (
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// doshamProbe is a fake dosham that records what was asked, how often, and how
// many lookups were in flight at once. primaries names the words the bot
// searches first; everything else is a retry and counts against the retry pool.
type doshamProbe struct {
	primaries map[string]bool
	// entries maps a searched word to the headword it resolves to; missing
	// words answer empty.
	entries map[string]string
	// hold, when non-nil, blocks every retry until it is closed.
	hold <-chan struct{}

	mu        sync.Mutex
	calls     map[string]int
	inFlight  int
	peakRetry int
	delay     time.Duration
}

func (p *doshamProbe) start(t *testing.T) {
	t.Helper()
	p.calls = map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Variables struct {
				InputText string `json:"inputText"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		word := req.Variables.InputText
		retry := !p.primaries[word]

		p.mu.Lock()
		p.calls[word]++
		if retry {
			p.inFlight++
			if p.inFlight > p.peakRetry {
				p.peakRetry = p.inFlight
			}
		}
		delay := p.delay
		p.mu.Unlock()

		if retry && p.hold != nil {
			select {
			case <-p.hold:
			case <-r.Context().Done():
			}
		}
		if delay > 0 {
			time.Sleep(delay)
		}

		if retry {
			p.mu.Lock()
			p.inFlight--
			p.mu.Unlock()
		}

		find := "[]"
		if headword, ok := p.entries[word]; ok {
			find = fmt.Sprintf(`[{"entryId":"e1","content":%q,"type":"WORD","translations":[{"translationId":"t1","content":"перевод","languageCode":"ru"}]}]`, headword)
		}
		fmt.Fprintf(w, `{"data":{"find":%s}}`, find)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DOSHAM_API_URL", srv.URL)
}

func (p *doshamProbe) count(word string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[word]
}

func (p *doshamProbe) peak() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.peakRetry
}

// A miss on a Chechen-looking query retries the palochka, not ё/е, and the
// dropped spelling reaches the real headword.
func TestFetchWithFallback_PalochkaRetryFindsTheWord(t *testing.T) {
	probe := &doshamProbe{
		primaries: map[string]bool{"чегӏардиг": true},
		entries:   map[string]string{"чӏегӏардиг": "чӏегӏардиг"},
	}
	probe.start(t)
	b := &Business{log: logrus.New()}

	got, err := b.fetchTranslationsWithFallback("чегӏардиг")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 1 || got[0].Original != "чӏегӏардиг" {
		t.Fatalf("got %+v, want the palochka spelling", got)
	}
}

// Ten people typing the same typo at the same time must cost one cascade, not
// ten. The cascade inside still makes its own requests — singleflight collapses
// cascades, not the lookups within one.
func TestTranslate_SingleflightCollapsesConcurrentMisses(t *testing.T) {
	probe := &doshamProbe{
		primaries: map[string]bool{"чегӏардиг": true},
		// Long enough that every caller is inside the flight before the first
		// one finishes and reopens the key.
		delay: 300 * time.Millisecond,
	}
	probe.start(t)
	b := &Business{log: logrus.New(), cache: cache.NewCache("127.0.0.1:1", "")}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			<-start
			b.Translate("чегӏардиг")
		})
	}
	close(start)
	wg.Wait()

	if n := probe.count("чегӏардиг"); n != 1 {
		t.Fatalf("primary lookup ran %d times, want 1", n)
	}
}

// The retry pool is a hard ceiling: however many cascades pile up, dosham never
// sees more than maxRetryRequests respellings at once.
func TestRetrySemaphore_CapsConcurrentRetries(t *testing.T) {
	words := []string{"цӏацӏацӏацӏа", "гӏагӏагӏагӏа", "бӏабӏабӏабӏа", "тӏатӏатӏатӏа", "кӏакӏакӏакӏа"}
	primaries := map[string]bool{}
	for _, w := range words {
		primaries[w] = true
	}
	probe := &doshamProbe{primaries: primaries, delay: 20 * time.Millisecond}
	probe.start(t)
	b := &Business{log: logrus.New()}

	var wg sync.WaitGroup
	for _, w := range words {
		wg.Go(func() { b.fetchTranslationsWithFallback(w) })
	}
	wg.Wait()

	if peak := probe.peak(); peak > maxRetryRequests {
		t.Fatalf("%d retries were in flight at once, cap is %d", peak, maxRetryRequests)
	}
	if probe.peak() < 2 {
		t.Fatal("the cascade never ran concurrently — the test proves nothing")
	}
}

// A fresh first lookup must not queue behind somebody else's cascade. Primary
// requests take no slot from the retry pool, so a full pool cannot delay them.
func TestPrimaryLookup_DoesNotWaitOnRetryPool(t *testing.T) {
	hold := make(chan struct{})
	probe := &doshamProbe{
		primaries: map[string]bool{"цӏацӏацӏацӏа": true, "яблоко": true},
		entries:   map[string]string{"яблоко": "Яблоко"},
		hold:      hold,
	}
	probe.start(t)
	b := &Business{log: logrus.New()}

	cascade := make(chan struct{})
	go func() {
		defer close(cascade)
		b.fetchTranslationsWithFallback("цӏацӏацӏацӏа")
	}()

	// Wait until the retry pool is saturated.
	deadline := time.Now().Add(2 * time.Second)
	for probe.peak() < maxRetryRequests {
		if time.Now().After(deadline) {
			close(hold)
			<-cascade
			t.Skip("cascade never filled the retry pool on this machine")
		}
		time.Sleep(time.Millisecond)
	}

	done := make(chan error, 1)
	go func() {
		_, err := b.fetchTranslationsFromAPI("яблоко")
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("primary lookup failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("primary lookup blocked while the retry pool was full")
	}

	close(hold)
	<-cascade
}

type foldedDictRepo struct {
	recordingDictRepo
	byFolded map[string][]models.TranslationPairs
}

func (r *foldedDictRepo) FindTranslationPairsByFolded(_ context.Context, folded string, _ int) ([]models.TranslationPairs, error) {
	return r.byFolded[folded], nil
}

// Layer 1: a word already in the local table answers a palochka-less query
// without touching dosham at all.
func TestTranslate_FoldedLocalHitSkipsTheAPI(t *testing.T) {
	probe := &doshamProbe{primaries: map[string]bool{"чегардиг": true}}
	probe.start(t)

	repo := &foldedDictRepo{byFolded: map[string][]models.TranslationPairs{
		"чегардиг": {{Original: "Чӏегӏардиг", Translate: "Ласточка", OriginalLang: "CHE", TranslateLang: "RUS"}},
	}}
	b := &Business{log: logrus.New(), cache: cache.NewCache("127.0.0.1:1", ""), dictRepo: repo}

	got, err := b.Translate("чегардиг")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if len(got) != 1 || got[0].Original != "Чӏегӏардиг" {
		t.Fatalf("got %+v, want the stored headword", got)
	}
	if n := probe.count("чегардиг"); n != 0 {
		t.Fatalf("the API was queried %d times for a word we already had", n)
	}
}

// The cascade must stop at the respelling that answers. It judges that by the
// folded key: normalizeForRank keeps the palochka, so «чӏегӏардиг» never reads
// as an exact match for «чегӏардиг» and the cascade used to run to the end,
// merging every candidate's substring hits into the card.
func TestCascade_StopsAtTheMatchAndDropsTheRest(t *testing.T) {
	probe := &doshamProbe{
		primaries: map[string]bool{"чегӏардиг": true},
		entries: map[string]string{
			"чӏегӏардиг": "чӏегӏардиг", // the real word, second candidate
			"чегӏардӏиг": "мусор",      // later candidates return unrelated hits
			"чегӏардигӏ": "мусор",
		},
		delay: 30 * time.Millisecond,
	}
	probe.start(t)
	b := &Business{log: logrus.New()}

	got, err := b.fetchTranslationsWithFallback("чегӏардиг")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("the real word was not found")
	}
	for _, p := range got {
		if p.Original == "мусор" {
			t.Fatalf("cascade ran past its answer and merged junk: %+v", got)
		}
	}
}

// The word the user was actually looking for has to lead its own card. A folded
// hit is not an exact hit by the strict key, so without its own bucket it tied
// at the bottom with unrelated substring matches.
func TestRankPair_FoldedHitOutranksUnrelated(t *testing.T) {
	key := normalizeForRank("чегардиг")
	folded := models.TranslationPairs{Original: "Чӏегӏардиг", Translate: "ласточка"}
	unrelated := models.TranslationPairs{Original: "Чегарда", Translate: "что-то"}
	if rankPair(folded, key) >= rankPair(unrelated, key) {
		t.Fatalf("folded hit ranks %d, unrelated ranks %d — the answer does not lead",
			rankPair(folded, key), rankPair(unrelated, key))
	}
	// An exact hit still beats a folded one.
	exact := models.TranslationPairs{Original: "чегардиг", Translate: "x"}
	if rankPair(exact, key) >= rankPair(folded, key) {
		t.Fatal("folded matching displaced the exact match")
	}
}
