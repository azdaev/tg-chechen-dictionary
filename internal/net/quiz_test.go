package net

import "testing"

func TestQuizPromptFromMessage(t *testing.T) {
	cases := []struct {
		text, want string
	}{
		{"🧠 Викторина\n\nКак переводится на русский?\n\nдошам", "дошам"},
		{"🧠 Викторина\n\nКак сказать по-чеченски?\n\nкӀант ", "кӀант"},
		{"одна строка", "одна строка"},
		{"", ""},
	}
	for _, c := range cases {
		if got := quizPromptFromMessage(c.text); got != c.want {
			t.Errorf("quizPromptFromMessage(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}
