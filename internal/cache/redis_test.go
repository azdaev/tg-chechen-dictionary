package cache

import (
	"strconv"
	"strings"
	"testing"
)

func TestTranslationKeyIsVersioned(t *testing.T) {
	key := translationKey("вода")
	if !strings.HasSuffix(key, "_вода") {
		t.Fatalf("translationKey(%q) = %q, want it to end in the clean word", "вода", key)
	}
	if !strings.Contains(key, strconv.Itoa(translationKeyVersion)) {
		t.Fatalf("translationKey(%q) = %q, want the version embedded so a bump changes the key", "вода", key)
	}
}

// The grammar cache must not share the translation version: routing both
// through one version would wipe every grammar entry on a format bump, and
// each grammar miss costs a live dosham query.
func TestGrammarKeyIndependentOfTranslationVersion(t *testing.T) {
	grammar := grammarKey("вода")
	if strings.Contains(grammar, "tr") {
		t.Fatalf("grammarKey(%q) = %q, want it free of the translation namespace", "вода", grammar)
	}
	if grammar == translationKey("вода") {
		t.Fatal("grammar and translation keys collide")
	}
}
