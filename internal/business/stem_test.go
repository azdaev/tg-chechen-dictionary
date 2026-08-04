package business

import (
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"chetoru/internal/repository"
	"chetoru/pkg/tools"
	"context"
	"testing"

	"github.com/sirupsen/logrus"
)

type stemDictRepo struct {
	recordingDictRepo
	byWord   map[string][]models.TranslationPairs
	byPrefix map[string][]models.TranslationPairs
}

func (r *stemDictRepo) FindTranslationPairs(_ context.Context, cleanWord string, _ int) ([]models.TranslationPairs, error) {
	return r.byWord[cleanWord], nil
}

func (r *stemDictRepo) FindTranslationPairsByPrefix(_ context.Context, prefix string, _ int) ([]models.TranslationPairs, error) {
	return r.byPrefix[prefix], nil
}

// The embedded fake posts every insert to a channel a test must drain; these
// tests care about the read path, so writes are dropped instead.
func (r *stemDictRepo) InsertTranslationPair(context.Context, repository.TranslationPair) (int64, bool, error) {
	return 0, false, nil
}

func newStemBusiness(repo *stemDictRepo) *Business {
	return &Business{log: logrus.New(), cache: cache.NewCache("127.0.0.1:1", ""), dictRepo: repo}
}

// «рука» answered and «руки» did not — dosham's search is a substring match, so
// an inflected Russian form reaches none of its own lemma's entries. The word
// forms layer above this one only knows paradigms dosham analyzed, and those are
// Chechen, so a Russian ending had no layer at all: the user was told the word
// does not exist and it was filed as a gap in a dictionary that holds it.
func TestTranslate_RussianEndingOpensTheLemma(t *testing.T) {
	probe := &doshamProbe{primaries: map[string]bool{"руки": true}}
	probe.start(t)

	repo := &stemDictRepo{
		byWord: map[string][]models.TranslationPairs{
			"рука": {{Original: "Рука", Translate: "м куьг", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100}},
		},
		byPrefix: map[string][]models.TranslationPairs{
			"рук": {
				{Original: "рукав", Translate: "пхьуьйш"},
				{Original: "рука", Translate: "куьг"},
			},
		},
	}
	b := newStemBusiness(repo)

	got, resolved, err := b.TranslateResolved("руки")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if len(got) != 1 || got[0].Original != "Рука" {
		t.Fatalf("got %+v, want the lemma's pairs", got)
	}
	if resolved != "рука" {
		t.Errorf("resolved = %q; the card renders against this and would come out empty", resolved)
	}
	if n := probe.count("руки"); n == 0 {
		t.Error("the lemma was served without asking dosham first, which would answer «столб» with «стол»")
	}
}

// «домов» stems to «домо» and to «дом». Taking the longest stem's first hit
// answers «домовой»; the shortest headword across every stem is «дом».
func TestTranslate_StemPrefersTheShortestLemma(t *testing.T) {
	probe := &doshamProbe{primaries: map[string]bool{"домов": true}}
	probe.start(t)

	repo := &stemDictRepo{
		byWord: map[string][]models.TranslationPairs{
			"дом": {{Original: "Дом", Translate: "м цӏа", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100}},
		},
		byPrefix: map[string][]models.TranslationPairs{
			"домо": {{Original: "домовой", Translate: "тарам"}},
			"дом":  {{Original: "домовой", Translate: "тарам"}, {Original: "дом", Translate: "цӏа"}},
		},
	}

	_, resolved, err := newStemBusiness(repo).TranslateResolved("домов")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if resolved != "дом" {
		t.Fatalf("resolved = %q, want дом", resolved)
	}
}

// A stem whose only neighbours are much longer words is not a lemma: «рукавица»
// is six letters past «рук» and a different word. Answering with it would be
// worse than the miss, which at least records a real gap.
func TestTranslate_DistantNeighbourIsNotALemma(t *testing.T) {
	probe := &doshamProbe{primaries: map[string]bool{"руки": true}}
	probe.start(t)

	repo := &stemDictRepo{
		byWord:   map[string][]models.TranslationPairs{},
		byPrefix: map[string][]models.TranslationPairs{"рук": {{Original: "рукавица", Translate: "мачаш"}}},
	}

	if _, resolved, err := newStemBusiness(repo).TranslateResolved("руки"); err != nil {
		t.Fatalf("translate: %v", err)
	} else if resolved != "" {
		t.Fatalf("resolved = %q; a distant neighbour was served as the lemma", resolved)
	}
	if n := probe.count("руки"); n == 0 {
		t.Error("dosham was never asked")
	}
}

// «столб» is not «стол» with an ending — it is a word, and dosham holds it. The
// stem guess must never pre-empt the dictionary: answering a real word with its
// shorter neighbour is worse than the miss it replaces.
func TestTranslate_StemNeverPreemptsARealWord(t *testing.T) {
	probe := &doshamProbe{
		primaries: map[string]bool{"столб": true},
		entries:   map[string]string{"столб": "Столб"},
	}
	probe.start(t)

	repo := &stemDictRepo{
		byWord:   map[string][]models.TranslationPairs{"стол": {{Original: "Стол", Translate: "м стол", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100}}},
		byPrefix: map[string][]models.TranslationPairs{"стол": {{Original: "стол", Translate: "стол"}}},
	}

	got, resolved, err := newStemBusiness(repo).TranslateResolved("столб")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if resolved != "" {
		t.Fatalf("resolved = %q; a word the dictionary holds was answered with its neighbour", resolved)
	}
	if len(got) == 0 || got[0].Original != "Столб" {
		t.Fatalf("got %+v, want the entry dosham holds for столб", got)
	}
}

// «гӏалгӏайн» typed without its palochkas is not a Russian word with an ending.
// prefixCandidates trims four letters — right for a labelled suggestion, wrong
// for an answer — so «галгайн» reached the stem «гал» and the bot replied with a
// card for «галоп», in the other language, with nothing marking it as a guess.
func TestTranslate_StemDoesNotAnswerAcrossWords(t *testing.T) {
	probe := &doshamProbe{primaries: map[string]bool{"галгайн": true}}
	probe.start(t)

	repo := &stemDictRepo{
		byWord:   map[string][]models.TranslationPairs{"галоп": {{Original: "Галоп", Translate: "м юм", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100}}},
		byPrefix: map[string][]models.TranslationPairs{"гал": {{Original: "галоп", Translate: "юм"}}},
	}

	got, resolved, err := newStemBusiness(repo).TranslateResolved("галгайн")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if resolved != "" || len(got) != 0 {
		t.Fatalf("answered %q with %+v; a lemma differs from the query by its ending, not by four letters", resolved, got)
	}
}

// A form can belong to more than one lemma, and the layer used to read them all
// and hand back one headword with everybody's pairs. The card renders against
// that single headword, so the other lemma's pairs match nothing in it and are
// dropped after the read was paid for — «лоьман» fetched «лоьма» to throw it
// away. One lemma answers: the first that holds anything.
func TestTranslate_FormAnswersWithOneLemma(t *testing.T) {
	repo := &formsDictRepo{
		byForm: map[string][]string{"лоьман": {"лом", "лоьма"}},
		byWord: map[string][]models.TranslationPairs{
			"лом":   {{Original: "лом", Translate: "лев", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 100}},
			"лоьма": {{Original: "лоьма", Translate: "другое", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 100}},
		},
	}

	b := &Business{log: logrus.New(), cache: cache.NewCache("127.0.0.1:1", ""), dictRepo: repo}
	got, resolved, err := b.TranslateResolved("лоьман")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if resolved != "лом" {
		t.Fatalf("resolved = %q, want лом", resolved)
	}
	if len(got) != 1 || got[0].Original != "лом" {
		t.Fatalf("got %+v, want only the answering lemma's pairs", got)
	}
}

// When the first lemma holds nothing, the next one answers.
func TestTranslate_FormSkipsAnEmptyLemma(t *testing.T) {
	repo := &formsDictRepo{
		byForm: map[string][]string{"лоьман": {"пусто", "лом"}},
		byWord: map[string][]models.TranslationPairs{
			"лом": {{Original: "лом", Translate: "лев", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 100}},
		},
	}
	b := &Business{log: logrus.New(), cache: cache.NewCache("127.0.0.1:1", ""), dictRepo: repo}
	if _, resolved, err := b.TranslateResolved("лоьман"); err != nil {
		t.Fatalf("translate: %v", err)
	} else if resolved != "лом" {
		t.Fatalf("resolved = %q, want лом", resolved)
	}
}

// dosham answers an inflected Russian form with the sentences that contain it
// and no entry: «собаку» comes back as six collocations about a dog and nothing
// that says what a dog is. Rows arrived, so the lemma layer below never ran, and
// the user read six illustrations of a word the bot never translated.
func TestTranslate_ExampleOnlyAnswerStillReachesTheLemma(t *testing.T) {
	probe := &doshamProbe{
		primaries: map[string]bool{"собаку": true},
		texts:     map[string]string{"собаку": "жӏаьла караӏамо"},
	}
	probe.start(t)

	repo := &stemDictRepo{
		byWord: map[string][]models.TranslationPairs{
			"собака": {{Original: "Собака", Translate: "ж жӏаьла", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, EntryType: "WORD", Rate: 100}},
		},
		byPrefix: map[string][]models.TranslationPairs{
			"собак": {{Original: "собака", Translate: "жӏаьла", OriginalLang: "RUS", TranslateLang: "CHE"}},
		},
	}
	b := newStemBusiness(repo)

	got, resolved, err := b.TranslateResolved("собаку")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if resolved != "собака" {
		t.Fatalf("resolved = %q, want the lemma: %+v", resolved, got)
	}
	if !tools.Render(resolved, got).Glossed {
		t.Errorf("the lemma's card still says nothing about the word: %+v", got)
	}
	b.WaitBackground()
}

// The other side of the same rule: when no lemma is better, the illustrations
// are the answer. «даться» is held only as «не даться в обман».
func TestTranslate_ExampleOnlyAnswerSurvivesWithoutALemma(t *testing.T) {
	probe := &doshamProbe{
		primaries: map[string]bool{"даться": true},
		texts:     map[string]string{"даться": "ӏеха ца вайта"},
	}
	probe.start(t)

	b := newStemBusiness(&stemDictRepo{})
	got, _, err := b.TranslateResolved("даться")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("the one line the dictionary holds was dropped for «нет перевода»")
	}
	b.WaitBackground()
}

// dosham's own search resolves an inflected Chechen form to its entry —
// find("кемсана") answers «кемс : виноград» — and the card dropped it, because
// the card only renders what the query appears in. The lemma was recovered
// afterwards by scanning the local table, which holds the entry only once the
// detached write of this very lookup has landed: on the first reading of the
// word the reader raced a goroutine for the answer. It is in the reply already.
func TestTranslate_LemmaComesOffWhatDoshamAnswered(t *testing.T) {
	probe := &doshamProbe{
		primaries: map[string]bool{"кемсана": true},
		entries:   map[string]string{"кемсана": "кемс"},
	}
	probe.start(t)

	// Nothing stored: no local pairs, no prefix index, nothing written yet.
	b := newStemBusiness(&stemDictRepo{})

	got, resolved, err := b.TranslateResolved("кемсана")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if resolved != "кемс" {
		t.Fatalf("resolved = %q, want the lemma dosham returned", resolved)
	}
	if len(got) == 0 || got[0].Original != "кемс" {
		t.Fatalf("got %+v, want the lemma's pairs", got)
	}
}

// Chechen marks noun class on the verb, so «ваха», «яха», «баха» and «даха» are
// one word; the dictionary spells out the д-form and files the rest under it
// with «см. даха». The card printed that pointer as if it were a translation —
// a dead end on 21 of 545 sampled Chechen entries, among them the most ordinary
// verbs there are.
func TestTranslate_FollowsSeeAlso(t *testing.T) {
	probe := &doshamProbe{
		primaries: map[string]bool{"ваха": true, "даха": true},
		entries:   map[string]string{"ваха": "ваха", "даха": "даха"},
		glosses:   map[string]string{"ваха": "см. даха", "даха": "жить"},
	}
	probe.start(t)
	b := newStemBusiness(&stemDictRepo{})

	got, resolved, err := b.TranslateResolved("ваха")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if resolved != "даха" {
		t.Fatalf("resolved = %q, want the entry the pointer names", resolved)
	}
	if len(got) == 0 || got[0].Translate != "жить" {
		t.Fatalf("got %+v, want the referred entry's translation", got)
	}
}

// Following the pointer must not cost a card that says something on its own:
// «мукъаниг» points at «мукъа» and is itself glossed «гласный».
func TestTranslate_KeepsACardThatAlsoTranslates(t *testing.T) {
	probe := &doshamProbe{
		primaries: map[string]bool{"мукъаниг": true},
		entries:   map[string]string{"мукъаниг": "мукъаниг"},
		glosses:   map[string]string{"мукъаниг": "см. мукъа"},
	}
	probe.start(t)
	repo := &stemDictRepo{byWord: map[string][]models.TranslationPairs{
		"мукъаниг": {
			{Original: "мукъаниг", Translate: "см. мукъа", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD"},
			{Original: "мукъаниг", Translate: "гласный", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD"},
		},
	}}

	_, resolved, err := newStemBusiness(repo).TranslateResolved("мукъаниг")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if resolved != "" {
		t.Errorf("resolved = %q; the word has a translation of its own", resolved)
	}
}
