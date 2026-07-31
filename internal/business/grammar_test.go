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
