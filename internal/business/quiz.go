package business

import (
	"chetoru/internal/models"

	"context"
	"math/rand/v2"
	"strings"
)

const quizOptionCount = 4

// GenerateQuiz builds a multiple-choice question with one correct option and
// several plausible distractors. Pairs come from the prefetched word pool —
// mutually distinct on both sides — so a question usually costs no API call.
// Half the questions reverse direction (Russian prompt, Chechen options):
// production recall is harder and more valuable than recognition. Active
// recall practice is the /quiz feature's engine.
func (b *Business) GenerateQuiz(ctx context.Context) (*models.QuizQuestion, error) {
	pairs, err := b.randomCleanWords(ctx, quizOptionCount)
	if err != nil {
		return nil, err
	}

	reversed := rand.IntN(2) == 0
	side := func(p models.RandomWord) string {
		if reversed {
			return p.Chechen
		}
		return p.Russian
	}

	question := pairs[0]
	prompt := question.Russian
	if !reversed {
		prompt = question.Chechen
	}

	options := make([]string, 0, quizOptionCount)
	for _, p := range pairs {
		options = append(options, side(p))
		if len(options) == quizOptionCount {
			break
		}
	}

	rand.Shuffle(len(options), func(i, j int) { options[i], options[j] = options[j], options[i] })

	correctIdx := 0
	for i, opt := range options {
		if opt == side(question) {
			correctIdx = i
			break
		}
	}

	return &models.QuizQuestion{
		Prompt:     prompt,
		Options:    options,
		CorrectIdx: correctIdx,
		Reversed:   reversed,
	}, nil
}

// isCleanMeaning reports whether a Russian gloss is a concise standalone answer
// suitable as a quiz option or /random card — not a multi-clause dictionary
// entry, a cross-reference ("см. ..."), or a derivational annotation
// ("понуд. от ...", "масд. от ...", "прил. ...").
func isCleanMeaning(russian string) bool {
	russian = strings.TrimSpace(russian)
	if russian == "" {
		return false
	}
	if strings.ContainsAny(russian, ";~") {
		return false
	}
	// A period signals a dictionary abbreviation ("понуд. от", "прил.", "см."),
	// never appearing in a plain Russian meaning of a single word.
	if strings.Contains(russian, ".") {
		return false
	}
	return len([]rune(russian)) <= 40
}
