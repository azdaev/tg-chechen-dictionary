package ai

import "testing"

func TestStripCodeFence(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"no fence", "дитт — дерево", "дитт — дерево"},
		{"plain fence", "```\nдитт — дерево\n```", "дитт — дерево"},
		{"language-tagged fence", "```text\nдитт — дерево\n```", "дитт — дерево"},
		{"single-line fence", "```дитт```", "дитт"},
		{"leading whitespace", "  \n```\nдитт\n```  ", "дитт"},
	}
	for _, c := range cases {
		if got := stripCodeFence(c.in); got != c.want {
			t.Errorf("%s: stripCodeFence(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestParseSpellCheck(t *testing.T) {
	if r := parseSpellCheck("NO_ERRORS"); !r.NoErrors {
		t.Error("bare NO_ERRORS not recognized")
	}
	if r := parseSpellCheck("```\nNO_ERRORS\n```"); !r.NoErrors {
		t.Error("fenced NO_ERRORS not recognized")
	}

	raw := "CORRECTED: дала безам бу\nCHANGES:\n• беза → безам"
	r := parseSpellCheck(raw)
	if r.NoErrors {
		t.Fatal("corrections misread as NO_ERRORS")
	}
	if r.Corrected != "дала безам бу" {
		t.Errorf("Corrected = %q, want %q", r.Corrected, "дала безам бу")
	}
	if r.Explanation != raw {
		t.Errorf("Explanation = %q, want the full response", r.Explanation)
	}

	// A free-form answer (e.g. "Это не чеченский текст") keeps the text as
	// explanation with no corrected form.
	r = parseSpellCheck("Это не чеченский текст")
	if r.NoErrors || r.Corrected != "" || r.Explanation == "" {
		t.Errorf("free-form result = %+v", r)
	}
}
