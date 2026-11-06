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
