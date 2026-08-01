package business

import (
	"chetoru/internal/cache"
	"context"
	"net/http"
	"testing"

	"github.com/sirupsen/logrus"
)

// Grammar entries are cached for 30 days, so an outage written down as "this
// word has no grammar" hides the paradigm for a month — far longer than the
// outage. GrammarFor must report the failure and write nothing.
func TestGrammarFor_TransportErrorIsNotCached(t *testing.T) {
	stubDoshamAPI(t, http.StatusInternalServerError, ``)
	b := &Business{log: logrus.New(), cache: cache.NewCache("127.0.0.1:1", "")}

	g, err := b.GrammarFor(context.Background(), "дог")
	if err == nil {
		t.Fatalf("outage must surface as an error, got %+v", g)
	}
	if g != nil {
		t.Fatalf("no grammar may accompany an error, got %+v", g)
	}

	// A word that genuinely has no analyzed entry is a real answer, cacheable.
	stubDoshamAPI(t, http.StatusOK, `{"data":{"find":[]}}`)
	if g, err := b.GrammarFor(context.Background(), "ыыыы"); err != nil || g != nil {
		t.Fatalf("got %+v (err %v), want a cacheable nil", g, err)
	}
}

func TestPosFromDetails(t *testing.T) {
	cases := []struct {
		name    string
		details string
		want    string
	}{
		// Real dosham payloads observed via the live API.
		{"noun", `{"Case":1,"Class":4,"NameType":0,"Declension":1,"NumeralType":0}`, "существительное"},
		{"verb", `{"Mood":1,"Class":0,"Tense":10,"Conjugation":1,"NumeralType":0,"Transitiveness":0}`, "глагол"},
		// Adjective/adverb schema is ambiguous without the legend -> no label.
		{"adjective schema", `{"Class":0,"Degree":null,"PluralCase":0,"SemanticType":2,"SingularCase":0,"Characteristic":1}`, ""},
		{"empty", "", ""},
		{"literal null", "null", ""},
		{"garbage", "not json", ""},
		{"unknown keys", `{"Foo":1}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := posFromDetails(c.details); got != c.want {
				t.Errorf("posFromDetails(%q) = %q, want %q", c.details, got, c.want)
			}
		})
	}
}

// A dosham lookup returns every entry whose content contains the query, so the
// card the user is reading competes with better-weighted neighbours. Picking by
// rate alone shows the translation of one word and the grammar of another.
func TestBestGrammarEntry_PrefersTheDisplayedHeadword(t *testing.T) {
	analyzed := func(content string, rate int) grammarEntry {
		return grammarEntry{Content: content, Type: "WORD", Rate: rate, Details: `{"Case":1,"Declension":1}`}
	}
	entries := []grammarEntry{
		analyzed("Диттан", 10000),
		analyzed("Дитт", 16),
		analyzed("Дитташ", 100),
	}

	if got := bestGrammarEntry(entries, "дитт"); got == nil || got.Content != "Дитт" {
		t.Fatalf("got %+v, want the headword the card shows despite its lower rate", got)
	}

	// Displayed headwords reach here spelled however the dictionary writes
	// them: palochka stand-ins, ё, a stress mark. EqualFold sees none of that.
	for _, spelling := range []string{"ДИТТ", "дитт "} {
		if got := bestGrammarEntry(entries, spelling); got == nil || got.Content != "Дитт" {
			t.Errorf("%q: got %+v, want Дитт", spelling, got)
		}
	}

	// No match is not "no card": the caller retries without a requirement, and
	// that fallback has to still find the best entry. A Russian headword never
	// matches an analyzed Chechen entry, which is every Russian query.
	if got := bestGrammarEntry(entries, "дом"); got != nil {
		t.Errorf("a pinned lookup must not drift, got %+v", got)
	}
	if got := bestGrammarEntry(entries, ""); got == nil || got.Content != "Диттан" {
		t.Errorf("fallback = %+v, want the highest-rated entry", got)
	}
}

// The preference must degrade to the old behaviour rather than to nothing.
// Only Chechen entries are analyzed, so a Russian query can never match its own
// headword — as a filter this would delete the grammar card for every Russian
// lookup, which is most of them.
func TestComputeGrammar_RussianQueryStillGetsACard(t *testing.T) {
	stubDoshamAPI(t, http.StatusOK, `{"data":{"find":[
		{"content":"Цӏа","type":"WORD","rate":100,"details":"{\"Case\":1,\"Declension\":1}",
		 "entryForms":[{"content":"цӏенош"}],"relatedEntries":[]}]}}`)
	b := &Business{log: logrus.New()}

	g, err := b.computeGrammar(context.Background(), "дом")
	if err != nil {
		t.Fatalf("computeGrammar: %v", err)
	}
	if g == nil || g.Headword != "Цӏа" {
		t.Fatalf("got %+v, want the analyzed Chechen entry", g)
	}
}

func TestEntryHasGrammar(t *testing.T) {
	cases := []struct {
		name string
		e    grammarEntry
		want bool
	}{
		{"has details", grammarEntry{Details: `{"Case":1}`}, true},
		{"has forms", grammarEntry{EntryForms: []grammarForm{{Content: "деган"}}}, true},
		{"null details no forms", grammarEntry{Details: "null"}, false},
		{"empty", grammarEntry{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := entryHasGrammar(&c.e); got != c.want {
				t.Errorf("entryHasGrammar(%+v) = %v, want %v", c.e, got, c.want)
			}
		})
	}
}
