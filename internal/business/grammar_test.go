package business

import "testing"

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
