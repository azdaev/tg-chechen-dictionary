package business

import (
	"chetoru/internal/models"

	"context"
	"fmt"
	"math/rand/v2"
	"strings"
)

const quizOptionCount = 4

// GenerateQuiz builds a multiple-choice question: a Chechen word with one
// correct Russian meaning and several plausible distractors, all drawn from
// random dictionary entries. Active recall practice is far more effective for
// learning vocabulary than passive lookup — this is the /quiz feature's engine.
func (b *Business) GenerateQuiz(ctx context.Context) (*models.QuizQuestion, error) {
	entries, err := b.fetchRandomEntries(ctx, 40)
	if err != nil {
		return nil, err
	}

	// Collect clean, distinct word→meaning pairs usable as quiz items.
	pairs := make([]models.RandomWord, 0, len(entries))
	seenWord := make(map[string]bool)
	seenMeaning := make(map[string]bool)
	for _, entry := range entries {
		word := orientEntry(entry)
		if word == nil || !isLearnableWord(word.Chechen) || !isCleanMeaning(word.Russian) {
			continue
		}
		wordKey := strings.ToLower(word.Chechen)
		meaningKey := strings.ToLower(word.Russian)
		if seenWord[wordKey] || seenMeaning[meaningKey] {
			continue
		}
		seenWord[wordKey] = true
		seenMeaning[meaningKey] = true
		pairs = append(pairs, *word)
	}

	if len(pairs) < quizOptionCount {
		return nil, fmt.Errorf("not enough quiz candidates: have %d, need %d", len(pairs), quizOptionCount)
	}

	rand.Shuffle(len(pairs), func(i, j int) { pairs[i], pairs[j] = pairs[j], pairs[i] })

	question := pairs[0]
	options := make([]string, 0, quizOptionCount)
	options = append(options, question.Russian)
	for _, p := range pairs[1:] {
		options = append(options, p.Russian)
		if len(options) == quizOptionCount {
			break
		}
	}

	rand.Shuffle(len(options), func(i, j int) { options[i], options[j] = options[j], options[i] })

	correctIdx := 0
	for i, opt := range options {
		if opt == question.Russian {
			correctIdx = i
			break
		}
	}

	return &models.QuizQuestion{
		Chechen:    question.Chechen,
		Options:    options,
		CorrectIdx: correctIdx,
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
